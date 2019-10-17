package common

import (
	"encoding/json"
	"os"
)

type AppConfig struct {
	CanvasRows      int
	CanvasCols      int
	CooldownSeconds int
	PaletteColors   []string

	RedisAddress  string
	RedisPassword string
	RedisDatabase int

	WebSocketAppAddr string
}

// Read configuration file. Panic on error
func MustReadAppConfig(path string) *AppConfig {
	file, _ := os.Open(path)
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()
	decoder := json.NewDecoder(file)
	config := AppConfig{}
	if err := decoder.Decode(&config); err != nil {
		panic(err)
	}
	return &config
}
