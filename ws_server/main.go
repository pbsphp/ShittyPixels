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

package main

import (
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
)

const (
	ListenAddr = "localhost:8765"
	CanvasRows = 50
	CanvasCols = 100
)

// Print error message with [ ERROR ] prefix and description.
func logError(description string, err error) {
	log.Println("[ ERROR ]: ", description, err)
}

// Canvas matrix. Items are colors.
var matrix = [CanvasRows][CanvasCols]string{}

// We store all websocket connections with active users. When someone has changed pixel color we iterate over
// `allConnections' and notify each user about changes.
type ConnectionInfo struct {
	// TODO: User info
}

var allConnections = make(map[*websocket.Conn]ConnectionInfo)

// Client request should be JSON with:
// method -- method name ("setPixelColor" for example).
// args -- additional args for method (may be nil). Different schema for each method.
type WebSocketRequestData struct {
	Method string                 `json:"method"`
	Args   map[string]interface{} `json:"args"`
}

// Server message is JSON with:
// kind -- kind of message ("pixelColor" for example).
// data -- some data for given kind of message. For "pixelColor" it would be {"x": x, "y": y, "color": color}.
type WebSocketResponseData struct {
	Kind string      `json:"kind"`
	Data interface{} `json:"data"`
}

// Pixel representation for transfer: coords and color.
type PixelInfo struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Color string `json:"color"`
}

// Convert map returned by json.Unmarshal to PixelInfo.
func argsToPixelInfo(args map[string]interface{}) (*PixelInfo, error) {
	rawX, ok := args["x"]
	if !ok {
		return nil, errors.New("expected 'x' key")
	}
	rawY, ok := args["y"]
	if !ok {
		return nil, errors.New("expected 'y' key")
	}
	rawColor, ok := args["color"]
	if !ok {
		return nil, errors.New("expected 'color' key")
	}
	x, ok := rawX.(float64)
	if !ok {
		return nil, errors.New("expected 'x':Number key")
	}
	y, ok := rawY.(float64)
	if !ok {
		return nil, errors.New("expected 'y':Number key")
	}
	color, ok := rawColor.(string)
	if !ok {
		return nil, errors.New("expected 'color':String key")
	}

	return &PixelInfo{
		X:     int(x),
		Y:     int(y),
		Color: color,
	}, nil
}

// Is error caused due to closed WebSocket connection.
// If websocket connection is closed, it's OK. Do not log it.
func isWsClosedOk(err error) bool {
	return websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure)
}

// Handle client requests.
func serve(w http.ResponseWriter, r *http.Request) {
	upgraderConfig := websocket.Upgrader{
		// Do not check origin. Allow all incoming connections. CSRFs are welcome.
		// TODO: Check origin by wildcard. E.g. instance-*.example.com:8765.
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	c, err := upgraderConfig.Upgrade(w, r, nil)
	if err != nil {
		logError("upgrade", err)
		return
	}
	defer func() {
		err := c.Close()
		if err != nil {
			logError("close connection", err)
		}
	}()

	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			if !isWsClosedOk(err) {
				logError("read", err)
			}
			delete(allConnections, c)
			return
		}

		wsMessage := WebSocketRequestData{}
		err = json.Unmarshal(message, &wsMessage)
		if err != nil {
			logError("unmarshal", err)
			continue
		}

		switch wsMessage.Method {
		case "setPixelColor":
			// User is changing pixel color.
			// Expected JSON:
			// {
			//     "method": "setPixelColor",
			//     "data": {
			//         "x": <X coordinate>,
			//         "y": <Y coordinate>,
			//         "color": "<new color>"
			//     }
			// }
			// All open connections get event notification:
			// {
			//     "kind": "pixelColor",
			//     "data": { (same) }
			// }

			pixel, err := argsToPixelInfo(wsMessage.Args)
			if err != nil {
				// Problems with user data. Just ignore.
				logError("unmarshal (data)", err)
				break
			}

			log.Printf("setPixelColor(x=%d, y=%d, color=%s)\n", pixel.X, pixel.Y, pixel.Color)

			matrix[pixel.Y][pixel.X] = pixel.Color

			wsResponse := WebSocketResponseData{
				Kind: "pixelColor",
				Data: pixel,
			}
			response, err := json.Marshal(&wsResponse)
			if err != nil {
				logError("marshal", err)
				break
			}

			// Notify all connections.
			// Also collect invalid connections and remove them from `allConnections' list.
			invalidConnections := make([]*websocket.Conn, 0, 1)
			for conn := range allConnections {
				err = conn.WriteMessage(mt, response)
				if err != nil {
					if !isWsClosedOk(err) {
						logError("read", err)
					}
					invalidConnections = append(invalidConnections, conn)
				}
			}

			isCurrentConnectionInvalid := false
			for _, conn := range invalidConnections {
				delete(allConnections, conn)

				if conn == c {
					isCurrentConnectionInvalid = true
				}
			}
			if isCurrentConnectionInvalid {
				return
			}

		case "connectMe":
			// New user is connected.
			// Expected JSON:
			// {
			//     "method": "connectMe"
			// }
			// User should get event:
			// {
			//     "kind": "allPixelsColors",
			//     "data": [
			//         {
			//             "x": 0, "y": 0, "color": <(0;0) pixel color>
			//         },
			//         ... (for each pixel)
			//     ]
			// }
			// TODO: Send cooldown info (user may open multiple tabs).

			log.Printf("connectMe()\n")

			_, ok := allConnections[c]
			if !ok {
				allConnections[c] = ConnectionInfo{}
			}

			pixelsData := [CanvasRows * CanvasCols]PixelInfo{}

			for y := 0; y < CanvasRows; y++ {
				for x := 0; x < CanvasCols; x++ {
					position := y*CanvasCols + x
					pixelsData[position] = PixelInfo{
						X:     x,
						Y:     y,
						Color: matrix[y][x],
					}
				}
			}

			wsResponse := WebSocketResponseData{
				Kind: "allPixelsColors",
				Data: pixelsData,
			}
			response, err := json.Marshal(&wsResponse)
			if err != nil {
				logError("marshal", err)
				break
			}

			err = c.WriteMessage(mt, response)
			if err != nil {
				if isWsClosedOk(err) {
					delete(allConnections, c)
				} else {
					logError("read", err)
				}
				return
			}

		default:
			logError("unsupported method", nil)
		}
	}
}

func main() {
	for y := 0; y < CanvasRows; y++ {
		for x := 0; x < CanvasCols; x++ {
			if (x+y)%2 == 0 {
				matrix[y][x] = "gray"
			} else {
				matrix[y][x] = "white"
			}
		}
	}

	http.HandleFunc("/", serve)
	log.Fatal(http.ListenAndServe(ListenAddr, nil))
}
