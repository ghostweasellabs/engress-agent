package tunnel

import (
	"fmt"
	"strings"
)

// ParseEndpointMapping parses "endpoint-id:port" or "endpoint-id:host:port".
func ParseEndpointMapping(raw string) (id, addr string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("empty endpoint mapping")
	}
	colon := strings.Index(raw, ":")
	if colon <= 0 || colon == len(raw)-1 {
		return "", "", fmt.Errorf("invalid endpoint mapping %q", raw)
	}
	id = raw[:colon]
	rest := raw[colon+1:]
	if strings.Contains(rest, ":") {
		addr = rest
	} else {
		addr = "127.0.0.1:" + rest
	}
	return id, addr, nil
}

// EndpointLocalsFromFlags builds an endpoint-id → local address map from CLI values.
func EndpointLocalsFromFlags(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for _, v := range values {
		id, addr, err := ParseEndpointMapping(v)
		if err != nil {
			return nil, err
		}
		if _, exists := out[id]; exists {
			return nil, fmt.Errorf("duplicate endpoint id %q", id)
		}
		out[id] = addr
	}
	return out, nil
}
