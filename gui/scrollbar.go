package gui

import (
	"github.com/go-gl/gl/all-core/gl"
	"github.com/go-gl/glfw/v3.2/glfw"
)

const (
	scrollbarVertexShaderSource = `
		#version 330 core
		layout (location = 0) in vec2 position;
		uniform vec2 resolution;

		void main() {
			// convert from window coordinates to GL coordinates
			vec2 glCoordinates = ((position / resolution) * 2.0 - 1.0) * vec2(1, -1);

			gl_Position = vec4(glCoordinates, 0.0, 1.0);
		}` + "\x00"

	scrollbarFragmentShaderSource = `
		#version 330 core
		uniform vec4 inColor;
		out vec4 outColor;
		void main() {
			outColor = inColor;
		}` + "\x00"

	BorderVertexValuesCount = 16
	ArrowsVertexValuesCount = 24
)

type scrollbarPart int

const (
	UpperArrow scrollbarPart = iota
	UpperSpace               // the space between upper arrow and thumb
	Thumb
	BottomSpace // the space between thumb and bottom arrow
	BottomArrow
)

type ScreenRectangle struct {
	left, top     float32 // upper left corner in pixels relative to the window (in pixels)
	right, bottom float32
}

func (sr *ScreenRectangle) width() float32 {
	return sr.right - sr.left
}

func (sr *ScreenRectangle) height() float32 {
	return sr.bottom - sr.top
}

func (sr *ScreenRectangle) isInside(x float32, y float32) bool {
	return x >= sr.left && x < sr.right &&
		y >= sr.top && y < sr.bottom
}

type scrollbar struct {
	program                   uint32
	vbo                       uint32
	vao                       uint32
	uniformLocationResolution int32
	uniformLocationInColor    int32

	position            ScreenRectangle // relative to the window's top left corner, in pixels
	positionUpperArrow  ScreenRectangle // relative to the control's top left corner
	positionBottomArrow ScreenRectangle
	positionThumb       ScreenRectangle

	scrollPosition    int
	maxScrollPosition int

	thumbIsDragging           bool
	startedDraggingAtPosition int     // scrollPosition when the dragging was started
	startedDraggingAtThumbTop float32 // sb.positionThumb.top when the dragging was started
	offsetInThumbY            float32 // y offset inside the thumb of the dragging point
	scrollPositionDelta       int
}

// Returns the vertical scrollbar width in pixels
func getDefaultScrollbarWidth() int {
	return 20 //400
}

func createScrollbarProgram() (uint32, error) {
	vertexShader, err := compileShader(scrollbarVertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		return 0, err
	}
	defer gl.DeleteShader(vertexShader)

	fragmentShader, err := compileShader(scrollbarFragmentShaderSource, gl.FRAGMENT_SHADER)
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

func newScrollbar() (*scrollbar, error) {
	prog, err := createScrollbarProgram()
	if err != nil {
		return nil, err
	}

	var vbo uint32
	var vao uint32

	gl.GenBuffers(1, &vbo)
	gl.GenVertexArrays(1, &vao)

	gl.BindVertexArray(vao)

	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, (BorderVertexValuesCount+ArrowsVertexValuesCount)*4, nil, gl.DYNAMIC_DRAW) // only reserve data

	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 2*4, nil)
	gl.EnableVertexAttribArray(0)

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindVertexArray(0)

	result := &scrollbar{
		program:                   prog,
		vbo:                       vbo,
		vao:                       vao,
		uniformLocationResolution: gl.GetUniformLocation(prog, gl.Str("resolution\x00")),
		uniformLocationInColor:    gl.GetUniformLocation(prog, gl.Str("inColor\x00")),

		position: ScreenRectangle{
			right:  0,
			bottom: 0,
			left:   0,
			top:    0,
		},

		scrollPosition:    0,
		maxScrollPosition: 0,

		thumbIsDragging: false,
	}

	result.recalcElementPositions()

	return result, nil
}

func (sb *scrollbar) Free() {
	if sb.program != 0 {
		gl.DeleteProgram(sb.program)
		sb.program = 0
	}

	if sb.vbo != 0 {
		gl.DeleteBuffers(1, &sb.vbo)
		sb.vbo = 0
	}

	if sb.vao != 0 {
		gl.DeleteBuffers(1, &sb.vao)
		sb.vao = 0
	}
}

// Recalc positions of the scrollbar elements according to current
func (sb *scrollbar) recalcElementPositions() {
	arrowHeight := sb.position.width()

	sb.positionUpperArrow = ScreenRectangle{
		left:   0,
		top:    0,
		right:  sb.position.width(),
		bottom: arrowHeight,
	}

	sb.positionBottomArrow = ScreenRectangle{
		left:   sb.positionUpperArrow.left,
		top:    sb.position.height() - arrowHeight,
		right:  sb.positionUpperArrow.right,
		bottom: sb.position.height(),
	}
	thumbHeight := sb.position.width()
	thumbTop := arrowHeight
	if sb.maxScrollPosition != 0 {
		thumbTop += (float32(sb.scrollPosition) * (sb.position.height() - thumbHeight - arrowHeight*2)) / float32(sb.maxScrollPosition)
	}

	sb.positionThumb = ScreenRectangle{
		left:   0,
		top:    thumbTop,
		right:  sb.position.width(),
		bottom: thumbTop + thumbHeight,
	}
}

func (sb *scrollbar) resize(gui *GUI) {
	sb.position.left = float32(gui.width) - float32(getDefaultScrollbarWidth())*gui.dpiScale
	sb.position.top = float32(1.0)
	sb.position.right = float32(gui.width)
	sb.position.bottom = float32(gui.height - 1)

	sb.recalcElementPositions()
}

func (sb *scrollbar) render(gui *GUI) {
	var savedProgram int32
	gl.GetIntegerv(gl.CURRENT_PROGRAM, &savedProgram)
	defer gl.UseProgram(uint32(savedProgram))

	gl.UseProgram(sb.program)
	gl.Uniform2f(sb.uniformLocationResolution, float32(gui.width), float32(gui.height))
	gl.Uniform4f(sb.uniformLocationInColor, 0.50, 0.50, 0.50, 1.0)

	borderVertices := [BorderVertexValuesCount]float32{
		sb.position.left, sb.position.top, sb.position.right, sb.position.top,
		sb.position.right, sb.position.top, sb.position.right, sb.position.bottom,
		sb.position.right, sb.position.bottom, sb.position.left, sb.position.bottom,
		sb.position.left, sb.position.bottom, sb.position.left, sb.position.top,
	}

	arrowVertices := [ArrowsVertexValuesCount]float32{
		// upper arrow
		sb.position.left + sb.positionUpperArrow.left, sb.position.top + sb.positionUpperArrow.bottom,
		sb.position.left + sb.positionUpperArrow.width()/2.0, sb.position.top + sb.positionUpperArrow.top,
		sb.position.left + sb.positionUpperArrow.right, sb.position.top + sb.positionUpperArrow.bottom,

		// bottom arrow
		sb.position.left + sb.positionBottomArrow.left, sb.position.top + sb.positionBottomArrow.top,
		sb.position.left + sb.positionBottomArrow.width()/2.0, sb.position.top + sb.positionBottomArrow.bottom,
		sb.position.left + sb.positionBottomArrow.right, sb.position.top + sb.positionBottomArrow.top,

		// thumb
		sb.position.left + sb.positionThumb.left, sb.position.top + sb.positionThumb.top,
		sb.position.left + sb.positionThumb.right, sb.position.top + sb.positionThumb.top,
		sb.position.left + sb.positionThumb.right, sb.position.top + sb.positionThumb.bottom,

		sb.position.left + sb.positionThumb.right, sb.position.top + sb.positionThumb.bottom,
		sb.position.left + sb.positionThumb.left, sb.position.top + sb.positionThumb.bottom,
		sb.position.left + sb.positionThumb.left, sb.position.top + sb.positionThumb.top,
	}

	gl.BindVertexArray(sb.vao)

	gl.NamedBufferSubData(sb.vbo, 0, len(borderVertices)*4, gl.Ptr(&borderVertices[0]))
	gl.DrawArrays(gl.LINES, 0, int32(len(borderVertices)/2))

	gl.NamedBufferSubData(sb.vbo, BorderVertexValuesCount*4, len(arrowVertices)*4, gl.Ptr(&arrowVertices[0]))
	gl.DrawArrays(gl.TRIANGLES, 2, ArrowsVertexValuesCount)

	gl.BindVertexArray(0)
}

func (sb *scrollbar) setPosition(max int, position int) {
	if max <= 0 {
		max = position
	}

	if position > max {
		position = max
	}

	sb.maxScrollPosition = max
	sb.scrollPosition = position

	sb.recalcElementPositions()
}

func (sb *scrollbar) mouseHitTest(px float64, py float64) scrollbarPart {
	// convert to local coordinates
	mouseX := float32(px - float64(sb.position.left))
	mouseY := float32(py - float64(sb.position.top))

	result := Thumb

	if sb.positionUpperArrow.isInside(mouseX, mouseY) {
		result = UpperArrow
	} else if sb.positionBottomArrow.isInside(mouseX, mouseY) {
		result = BottomArrow
	} else {
		// construct UpperSpace
		pos := ScreenRectangle{
			left:   sb.positionThumb.left,
			top:    sb.positionUpperArrow.bottom,
			right:  sb.positionThumb.right,
			bottom: sb.positionThumb.top,
		}

		if pos.isInside(mouseX, mouseY) {
			result = UpperSpace
		}

		// now update it to be BottomSpace
		pos.top = sb.positionThumb.bottom
		pos.bottom = sb.positionBottomArrow.top
		if pos.isInside(mouseX, mouseY) {
			result = BottomSpace
		}
	}

	return result
}

func (sb *scrollbar) isMouseInside(px float64, py float64) bool {
	return sb.position.isInside(float32(px), float32(py))
}

func (sb *scrollbar) mouseButtonCallback(g *GUI, button glfw.MouseButton, action glfw.Action, mod glfw.ModifierKey, mouseX float64, mouseY float64) {
	if button == glfw.MouseButtonLeft {
		if action == glfw.Press {
			switch sb.mouseHitTest(mouseX, mouseY) {
			case UpperArrow:
				g.terminal.ScreenScrollUp(1)

			case UpperSpace:
				g.terminal.ScrollPageUp()

			case Thumb:
				sb.thumbIsDragging = true
				sb.startedDraggingAtPosition = sb.scrollPosition
				sb.startedDraggingAtThumbTop = sb.positionThumb.top
				sb.offsetInThumbY = float32(mouseY) - sb.position.top - sb.positionThumb.top
				sb.scrollPositionDelta = 0

			case BottomSpace:
				g.terminal.ScrollPageDown()

			case BottomArrow:
				g.terminal.ScreenScrollDown(1)
			}
		} else if action == glfw.Release && sb.thumbIsDragging {
			sb.thumbIsDragging = false
		}
	}
}

func (sb *scrollbar) mouseMoveCallback(g *GUI, px float64, py float64) {
	if sb.thumbIsDragging {
		py -= float64(sb.position.top)

		minThumbTop := sb.positionUpperArrow.bottom
		maxThumbTop := sb.positionBottomArrow.top - sb.positionThumb.height()

		newThumbTop := float32(py) - sb.offsetInThumbY

		newPositionDelta := int((float32(sb.maxScrollPosition) * (newThumbTop - minThumbTop - sb.startedDraggingAtThumbTop)) / (maxThumbTop - minThumbTop))

		if newPositionDelta > sb.scrollPositionDelta {
			scrollLines := newPositionDelta - sb.scrollPositionDelta
			g.logger.Debugf("old position: %d, new position delta: %d, scroll down %d lines", sb.scrollPosition, newPositionDelta, scrollLines)
			g.terminal.ScreenScrollDown(uint16(scrollLines))
			sb.scrollPositionDelta = newPositionDelta
		} else if newPositionDelta < sb.scrollPositionDelta {
			scrollLines := sb.scrollPositionDelta - newPositionDelta
			g.logger.Debugf("old position: %d, new position delta: %d, scroll up %d lines", sb.scrollPosition, newPositionDelta, scrollLines)
			g.terminal.ScreenScrollUp(uint16(scrollLines))
			sb.scrollPositionDelta = newPositionDelta
		}

		sb.recalcElementPositions()
		g.logger.Debugf("new thumbTop: %f, fact thumbTop: %f, position: %d", newThumbTop, sb.positionThumb.top, sb.scrollPosition)
	}
}
