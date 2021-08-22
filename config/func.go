package config

import (
	"fmt"
	"github.com/rs/zerolog/log"
	"os"
)

func Set(key string, value string) {
	base[key] = value
}

func Get(key string) string {
	value := os.Getenv(key)
	if value != "" {
		return value
	}

	value = base[key]
	if value != "" {
		return value
	}

	log.Error().Str("file", "func").Msg(fmt.Sprintf("%v is empty", key))
	os.Exit(1)

	return ""
}

func mergeConfig(configs ...map[string]string) map[string]string {
	result := map[string]string{}

	for _, configMap := range configs {
		for key, configValue := range configMap {
			if _, ok := result[key]; ok {
				log.Error().Str("file", "func").Msg(fmt.Sprintf(`duplicate config key "%s" detected`, key))
			}

			result[key] = configValue
		}
	}

	return result
}
