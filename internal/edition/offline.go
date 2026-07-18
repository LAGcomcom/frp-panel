//go:build offline

package edition

import "github.com/frp-panel/frp-panel/internal/config"

const Offline = true

func Apply(cfg *config.Config) {
	if cfg == nil {
		return
	}
	cfg.Update = config.UpdateConfig{}
}
