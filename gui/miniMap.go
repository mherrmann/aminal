package gui

import (
	"github.com/go-gl/gl/all-core/gl"
	"github.com/liamg/aminal/glfont"
	"strings"
	"github.com/liamg/aminal/config"
	"github.com/liamg/aminal/buffer"
)

const (
	minimapVertexShaderSource = `
		#version 330 core
		layout (location = 0) in vec2 position;
		uniform vec2 resolution;

		void main() {
			// convert from window coordinates to GL coordinates 
			vec2 glCoordinates = ((position / resolution) * 2.0 - 1.0) * vec2(1, -1);

			gl_Position = vec4(glCoordinates, 0.0, 1.0);
		}` + "\x00"

	minimapFragmentShaderSource = `
		#version 330 core
		out vec4 outColor;
		void main() {
			outColor = vec4(1.0f, 0.5f, 0.2f, 0.5f);
		}` + "\x00"
)

type miniMap struct {
	program                   uint32
	vbo                       uint32
	vao                       uint32
	ibo                       uint32
	uniformLocationResolution int32

	fontMap *FontMap

	left, top      float32   // upper left corner in pixels relative to the window
	width, height  float32   // in pixels
	cellWidth      float32
	lineHeight     float32
}

func createMiniMapProgram() (uint32, error) {
	vertexShader, err := compileShader(minimapVertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		return 0, err
	}
	defer gl.DeleteShader(vertexShader)

	fragmentShader, err := compileShader(minimapFragmentShaderSource, gl.FRAGMENT_SHADER)
	if err != nil {
		return 0, err
	}
	defer gl.DeleteShader(fragmentShader)

	prog := gl.CreateProgram()
	gl.AttachShader(prog, vertexShader)
	gl.AttachShader(prog, fragmentShader)
	gl.LinkProgram(prog)

	return prog, nil
}

func newMiniMap() (*miniMap, error) {
	prog, err := createMiniMapProgram()
	if err != nil {
		return nil, err
	}

	var vbo uint32
	var vao uint32
	var ibo uint32

	gl.GenBuffers(1, &vbo)
	gl.GenVertexArrays(1, &vao)
	gl.GenBuffers(1, &ibo)

	vertices := [12]float32 {}

	indices := [...]uint32 {
		0, 1,
		1, 2,
		2, 3,
		3, 0,
	}

	gl.BindVertexArray(vao)

	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices) * 4, gl.Ptr(&vertices[0]), gl.DYNAMIC_DRAW)

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ibo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices) * 4, gl.Ptr(&indices[0]), gl.DYNAMIC_DRAW)

	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 2 * 4, nil)
	gl.EnableVertexAttribArray(0)

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindVertexArray(0)

	return &miniMap{
		program:                   prog,
		vbo:                       vbo,
		vao:                       vao,
		ibo:                       ibo,
		uniformLocationResolution: gl.GetUniformLocation(prog, gl.Str("resolution\x00")),
	}, nil
}

func (m *miniMap) Free() {
	if m.program != 0 {
		gl.DeleteProgram(m.program)
		m.program = 0
	}

	if m.vbo != 0 {
		gl.DeleteBuffers(1, &m.vbo)
		m.vbo = 0
	}

	if m.vao != 0 {
		gl.DeleteVertexArrays(1, &m.vao)
		m.vao = 0
	}

	if m.ibo != 0 {
		gl.DeleteBuffers(1, &m.ibo)
		m.ibo = 0
	}
}

func (m *miniMap) render(gui *GUI) {
	savedPolygonMode := [2]int32{}
	var savedProgram int32

	gl.GetIntegerv(gl.POLYGON_MODE, &savedPolygonMode[0])
	gl.GetIntegerv(gl.CURRENT_PROGRAM, &savedProgram)

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	defer func() {
		gl.Disable(gl.BLEND)

		gl.UseProgram(uint32(savedProgram))
		gl.PolygonMode(gl.FRONT_AND_BACK, uint32(savedPolygonMode[0]))
	}()

	m.renderText(gui)

	gl.UseProgram(m.program)
	gl.Uniform2f(m.uniformLocationResolution, float32(gui.width), float32(gui.height))

	vertices := [...]float32 {
		 m.left,                      m.top,
		 m.left + m.width,            m.top,
		 m.left + m.width, m.top + m.height,
		 m.left,           m.top + m.height,
	}

	gl.NamedBufferSubData(m.vao, 0, len(vertices) * 4, gl.Ptr(&vertices[0]))
	gl.BindVertexArray(m.vao)

	gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
	gl.DrawElements(gl.LINES, 8, gl.UNSIGNED_INT, gl.PtrOffset(0))

	gl.BindVertexArray(0)
}

func (m *miniMap) resize(gui *GUI) {
	m.fontMap = gui.fontMap

	m.width = float32(gui.width) / 10.0
	m.height = float32(gui.height - 1)
	m.left = float32(gui.width) - m.width
	m.top = float32(1.0)

	defaultFont := m.fontMap.DefaultFont()
	m.lineHeight = defaultFont.LineHeight()
	m.cellWidth, _ = defaultFont.Size("X")
}

func (m *miniMap) drawCellText(text string, scale float32, col int, row int, alpha float32, color *config.Colour, bold bool) {
	var f *glfont.Font
	if bold {
		f = m.fontMap.BoldFont()
	} else {
		f = m.fontMap.DefaultFont()
	}

	f.SetColor(color[0], color[1], color[2], alpha)

	x := m.left + m.cellWidth * float32(col)
	y := m.top + m.lineHeight * scale * float32(row)
	f.PrintScaled(scale, x, y, text)
}

func (m *miniMap) drawCellBg(cell *buffer.Cell, col int, row int, color *config.Colour) {

}

func (m *miniMap) renderText(gui *GUI) {
	if m.fontMap == nil {
		return
	}

	lines := gui.terminal.ActiveBuffer().GetLines()
	if len(lines) == 0 {
		return
	}

	colCount := int(gui.terminal.ActiveBuffer().ViewWidth())
	scale := float32(0.3)

	for row, line := range lines {
		var builder strings.Builder
		builder.Grow(colCount * len(lines)) // reserve space

		bold := false
		dim := false
		var colourFg *config.Colour
		var colourBg *config.Colour
		cells := line.Cells()
		if len(cells) > 0 {
			clr := cells[0].Fg()
			colourFg = (*config.Colour)(&clr)
		}

		colToDraw := 0
		alpha := float32(1.0)
		for col := 0; col < colCount; col++ {
			if gui.terminal.ActiveBuffer().InSelection(uint16(col), uint16(row)) {
				colourBg = &gui.config.ColourScheme.Selection
			} else {
				colourBg = nil
			}

			if colourBg != nil || col < len(cells) {
				cell := gui.defaultCell
				if col < len(cells) {
					cell = &cells[col]
				}
				if colourBg == nil {
					clr := cell.Bg()
					colourBg = (*config.Colour)(&clr)
				}

				m.drawCellBg(cell, col, row, colourBg)
			}

			if col < len(cells) {
				cell := cells[col]
				cellFg := cell.Fg()
				if builder.Len() > 0 && ( cell.Attr().Dim != dim || cell.Attr().Bold != bold || !config.ColoursEqual(colourFg, (*config.Colour)(&cellFg)) ) {
					if dim {
						alpha = 0.5
					} else {
						alpha = 1.0
					}
					m.drawCellText(builder.String(), scale, colToDraw, row, alpha, colourFg, bold)
					colToDraw = col
					builder.Reset()
				}
				dim = cell.Attr().Dim
				colourFg = (*config.Colour)(&cellFg)
				bold = cell.Attr().Bold
				r := cell.Rune()
				if r == 0 {
					r = ' '
				}
				builder.WriteRune(r)
			}
		}
		if builder.Len() > 0 {
			if dim {
				alpha = 0.5
			} else {
				alpha = 1.0
			}
			m.drawCellText(builder.String(), scale, colToDraw, row, alpha, colourFg, bold)
		}
	}
}