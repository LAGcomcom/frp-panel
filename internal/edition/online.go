//go:build !offline

package edition

import "github.com/frp-panel/frp-panel/internal/config"

const Offline = false

func Apply(_ *config.Config) {}
