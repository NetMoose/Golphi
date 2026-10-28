package ui

import "github.com/NetMoose/Golphi/internal/backend"

// SetClientSize задаёт размер клиентской области без оконных декораций.
// Размеры — положительные целые logical units; настройка разрешена до Run.
func (f *Form) SetClientSize(width, height int) {
	f.guardAlive()
	f.guardGeometryPreparation()
	if width <= 0 || height <= 0 {
		panic("ui: client size must be positive")
	}
	f.bounds = backend.Bounds{Width: width, Height: height}
}

// SetBounds задаёт фиксированное положение относительно внутренней области
// Parent и минимальный размер в целых logical units. Native-тема может увеличить
// размер. Координаты неотрицательны, минимальные размеры положительны.
// Метод разрешён до Run; автоматического размещения и коррекции пересечений нет.
func (c *control) SetBounds(x, y, width, height int) {
	c.guardAlive()
	c.guardGeometryPreparation()
	if x < 0 || y < 0 || width <= 0 || height <= 0 {
		panic("ui: bounds require nonnegative coordinates and positive size")
	}
	c.bounds = backend.Bounds{X: x, Y: y, Width: width, Height: height}
}

func (c *component) guardGeometryPreparation() {
	if c.started || (c.owner != nil && c.owner.started) || (c.parent != nil && c.parent.started) {
		panic("ui: geometry can only be configured before Run")
	}
}
