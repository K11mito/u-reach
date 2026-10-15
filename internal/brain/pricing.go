package brain

import "strings"

// USD per million tokens, from Anthropic's model documentation. Cache writes
// use the 5-minute TTL rate (1.25x input), which is the cache the brain sets.
var prices = map[string]struct{ in, out, cacheRead float64 }{
	"claude-opus-5-5":   {4, 20, 0.20},
	"claude-opus-5":     {5, 25, 0.50},
	"claude-sonnet-5-5": {2, 10, 0.20},
	"claude-haiku-5-5":  {0.10, 0.50, 0.01},
	"claude-fable-5-1":  {10, 50, 0.25},
}

// CostUSD is what the call that produced m cost. The response's model may
// carry a date suffix, so the longest matching price prefix wins; an unknown
// model is priced as Opus 5.5, the default.
func (m Meta) CostUSD() float64 {
	p, best := prices["claude-opus-5-5"], ""
	for name, price := range prices {
		if strings.HasPrefix(m.Model, name) && len(name) > len(best) {
			p, best = price, name
		}
	}
	return (float64(m.InputTokens)*p.in + float64(m.OutputTokens)*p.out +
		float64(m.CacheRead)*p.cacheRead + float64(m.CacheWrite)*p.in*1.25) / 1e6
}
