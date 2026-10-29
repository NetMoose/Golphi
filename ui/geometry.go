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
// До Run доступен для всех контролов; после начала Run — только для живого
// отсоединённого поддерева (вся цепочка Parent не достигает Form), независимо
// от прежнего native-создания. Во время Run требуется UI-поток.
// Автоматического размещения и коррекции пересечений нет.
func (c *visual) SetBounds(x, y, width, height int) {
	c.guardAlive()
	guardPreparationThread()
	if c.attached() {
		c.guardGeometryPreparation()
	}
	if x < 0 || y < 0 || width <= 0 || height <= 0 {
		panic("ui: bounds require nonnegative coordinates and positive size")
	}
	c.bounds = backend.Bounds{X: x, Y: y, Width: width, Height: height}
}

func (c *component) guardGeometryPreparation() {
	for p := c.parent; p != nil; p = p.componentIdentity().parent {
		if p.componentIdentity().started {
			panic("ui: geometry can only be configured before Run")
		}
	}
	if c.started || (c.owner != nil && c.owner.componentIdentity().started) || (c.parent != nil && c.parent.componentIdentity().started) {
		panic("ui: geometry can only be configured before Run")
	}
}
