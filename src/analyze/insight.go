package analyze

import (
	"fmt"
	"slices"
)

type InsightType string

const (
	BoolInsight   InsightType = "bool"
	NumberInsight InsightType = "number"
)

type Insight struct {
	Type  InsightType `json:"type"`
	Value any         `json:"value"`
}

func NewInsight(insightType InsightType, value any) *Insight {
	return &Insight{
		Type:  insightType,
		Value: value,
	}
}

func (i *Insight) Resolver() InsightResolver {
	switch i.Type {
	case BoolInsight:
		return newBoolInsightResolver()
	case NumberInsight:
		return newNumberInsightResolver()
	}

	return nil
}

type InsightResolver interface {
	Value(segment *Segment, key string) any
}

type boolInsightResolver struct {
}

func newBoolInsightResolver() *boolInsightResolver {
	return &boolInsightResolver{}
}

func (b *boolInsightResolver) Value(segment *Segment, key string) any {
	values, has := GetInsight[bool](segment, key)
	if !has {
		return fmt.Sprint(false)
	}

	return slices.Contains(values, true)
}

type numberInsightResolver struct {
}

func newNumberInsightResolver() *numberInsightResolver {
	return &numberInsightResolver{}
}

func (n *numberInsightResolver) Value(segment *Segment, key string) any {
	values, has := GetInsight[int](segment, key)
	if !has {
		return fmt.Sprint(0)
	}

	sum := 0
	for _, value := range values {
		sum += value
	}

	return sum
}
