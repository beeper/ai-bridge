package embedding

import (
	"context"
	"math"
)

type Provider struct {
	id         string
	model      string
	embedQuery func(ctx context.Context, text string) ([]float64, error)
	embedBatch func(ctx context.Context, texts []string) ([][]float64, error)
}

func (p *Provider) ID() string {
	return p.id
}

func (p *Provider) Model() string {
	return p.model
}

func (p *Provider) EmbedQuery(ctx context.Context, text string) ([]float64, error) {
	if p.embedQuery == nil {
		return nil, nil
	}
	return p.embedQuery(ctx, text)
}

func (p *Provider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if p.embedBatch == nil {
		return nil, nil
	}
	return p.embedBatch(ctx, texts)
}

func NormalizeEmbedding(vec []float64) []float64 {
	if len(vec) == 0 {
		return vec
	}
	var sum float64
	for _, v := range vec {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			sum += v * v
		}
	}
	if sum <= 0 {
		return vec
	}
	mag := math.Sqrt(sum)
	if mag < 1e-10 {
		return vec
	}
	out := make([]float64, len(vec))
	for i, v := range vec {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			out[i] = 0
		} else {
			out[i] = v / mag
		}
	}
	return out
}
