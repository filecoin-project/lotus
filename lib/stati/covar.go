package stati

import "math"

type Covar struct {
	meanX float64
	meanY float64
	c     float64
	n     float64
	m2x   float64
	m2y   float64
}

func (cov1 *Covar) MeanX() float64 {
	return cov1.meanX
}

func (cov1 *Covar) MeanY() float64 {
	return cov1.meanY
}

func (cov1 *Covar) N() float64 {
	return cov1.n
}

func (cov1 *Covar) Covariance() float64 {
	return cov1.c / (cov1.n - 1)
}

func (cov1 *Covar) VarianceX() float64 {
	return cov1.m2x / (cov1.n - 1)
}

func (cov1 *Covar) StddevX() float64 {
	return math.Sqrt(cov1.VarianceX())
}

func (cov1 *Covar) VarianceY() float64 {
	return cov1.m2y / (cov1.n - 1)
}

func (cov1 *Covar) StddevY() float64 {
	return math.Sqrt(cov1.VarianceY())
}

func (cov1 *Covar) AddPoint(x, y float64) {
	cov1.n++

	dx := x - cov1.meanX
	cov1.meanX += dx / cov1.n
	dx2 := x - cov1.meanX
	cov1.m2x += dx * dx2

	dy := y - cov1.meanY
	cov1.meanY += dy / cov1.n
	dy2 := y - cov1.meanY
	cov1.m2y += dy * dy2

	cov1.c += dx * dy
}

func (cov1 *Covar) Combine(cov2 *Covar) {
	if cov1.n == 0 {
		*cov1 = *cov2
		return
	}
	if cov2.n == 0 {
		return
	}

	if cov1.n == 1 {
		cpy := *cov2
		cpy.AddPoint(cov2.meanX, cov2.meanY)
		*cov1 = cpy
		return
	}
	if cov2.n == 1 {
		cov1.AddPoint(cov2.meanX, cov2.meanY)
	}

	out := Covar{}
	out.n = cov1.n + cov2.n

	dx := cov1.meanX - cov2.meanX
	out.meanX = cov1.meanX - dx*cov2.n/out.n
	out.m2x = cov1.m2x + cov2.m2x + dx*dx*cov1.n*cov2.n/out.n

	dy := cov1.meanY - cov2.meanY
	out.meanY = cov1.meanY - dy*cov2.n/out.n
	out.m2y = cov1.m2y + cov2.m2y + dy*dy*cov1.n*cov2.n/out.n

	out.c = cov1.c + cov2.c + dx*dy*cov1.n*cov2.n/out.n
	*cov1 = out
}

func (cov1 *Covar) A() float64 {
	return cov1.Covariance() / cov1.VarianceX()
}
func (cov1 *Covar) B() float64 {
	return cov1.meanY - cov1.meanX*cov1.A()
}
func (cov1 *Covar) Correl() float64 {
	return cov1.Covariance() / cov1.StddevX() / cov1.StddevY()
}
