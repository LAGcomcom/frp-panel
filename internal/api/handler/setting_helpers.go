package handler

import "strings"

func settingBool(settings map[string]string, key string, defaultValue bool) bool {
	value, ok := settings[key]
	if !ok || strings.TrimSpace(value) == "" {
		return defaultValue
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return defaultValue
	}
}
