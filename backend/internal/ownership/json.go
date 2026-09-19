package ownership

import "encoding/json"

func jsonBytes(v map[string]any) ([]byte, error) {
	if v == nil {
		v = map[string]any{}
	}
	return json.Marshal(v)
}
