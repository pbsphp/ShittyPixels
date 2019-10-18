/*
   ShittyPixels
   Copyright © 2019  Pbsphp

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.

   You should have received a copy of the GNU General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package common

import (
	"encoding/json"
	"os"
)

type AppConfig struct {
	CanvasRows      int
	CanvasCols      int
	CooldownSeconds int

	PaletteColors []string
	InitialImage  string

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
