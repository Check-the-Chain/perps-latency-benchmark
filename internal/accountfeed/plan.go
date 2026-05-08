package accountfeed

import (
	"fmt"
	"net/http"

	"perps-latency-benchmark/internal/payload"
	"perps-latency-benchmark/internal/venues/confirmutil"
)

type PlanOptions struct {
	Key      string
	Venue    string
	IDField  string
	Required []string
}

type Plan struct {
	Venue   string
	Key     string
	Raw     map[string]any
	IDs     map[string]struct{}
	Order   string
	WSURL   string
	Missing string
}

func DecodePlan(built payload.Built, opts PlanOptions) (Plan, bool, error) {
	raw, ok := built.Metadata[opts.Key].(map[string]any)
	if !ok || confirmutil.Text(raw["venue"]) != opts.Venue {
		return Plan{}, false, nil
	}
	plan := Plan{
		Venue: opts.Venue,
		Key:   opts.Key,
		Raw:   raw,
		Order: confirmutil.Text(raw["order_type"]),
		WSURL: confirmutil.Text(raw["ws_url"]),
	}
	for _, key := range opts.Required {
		if confirmutil.Text(raw[key]) == "" {
			return Plan{}, true, fmt.Errorf("%s %s metadata missing %s", opts.Venue, opts.Key, key)
		}
	}
	if opts.IDField != "" {
		plan.IDs = confirmutil.IDSet(raw[opts.IDField])
		if len(plan.IDs) == 0 {
			return Plan{}, true, fmt.Errorf("%s %s metadata missing %s", opts.Venue, opts.Key, opts.IDField)
		}
	}
	return plan, true, nil
}

func (p Plan) Text(key string) string {
	return confirmutil.Text(p.Raw[key])
}

func (p Plan) Object(key string) map[string]any {
	return confirmutil.Object(p.Raw[key])
}

func (p Plan) Headers(key string) http.Header {
	headers := http.Header{}
	for headerKey, value := range p.Object(key) {
		headers.Set(headerKey, confirmutil.Text(value))
	}
	return headers
}
