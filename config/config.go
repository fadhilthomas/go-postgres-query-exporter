package config

var base = mergeConfig(
	logFileConfig,
	logLevelConfig,
	logTimezoneConfig,
	logIntervalConfig,
	rateLimitConfig,
	rateIntervalConfig,
)
