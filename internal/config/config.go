package config

import "github.com/kelseyhightower/envconfig"

type Config struct {
	Port              string `envconfig:"PORT" default:"8090"`
	RedisAddr         string `envconfig:"REDIS_ADDR" default:"localhost:6379"`
	RedisPassword     string `envconfig:"REDIS_PASSWORD"`
	PartnerServiceURL string `envconfig:"PARTNER_SERVICE_URL" default:"http://partner:8088"`
	PartnerID         string `envconfig:"PARTNER_ID"`
	PartnerSecret     string `envconfig:"PARTNER_AUTH_TOKEN"`
	RoomTTLHours      int    `envconfig:"ROOM_TTL_HOURS" default:"24"`
	LogLevel          string `envconfig:"LOG_LEVEL" default:"info"`
	MongoURI          string `envconfig:"MONGO_URI" default:"mongodb://localhost:27017"`
	MongoDB           string `envconfig:"MONGO_DB" default:"aliasb"`
}

func Load() (*Config, error) {
	var c Config
	return &c, envconfig.Process("", &c)
}
