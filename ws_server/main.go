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
	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	ListenAddr = "localhost:8765"
	CanvasRows = 50
	CanvasCols = 100

	CooldownSeconds = 5

	RedisAddr     = "localhost:6379"
	RedisPassword = ""
	RedisDatabase = 0
)

// Print error message with [ ERROR ] prefix and description.
func logError(description string, err error) {
	log.Println("[ ERROR ]: ", description, err)
}

// Client request should be JSON with:
// method -- method name ("setPixelColor" for example).
// args -- additional args for method (may be nil). Different schema for each method.
// sessionToken -- session token for user authentication.
type WebSocketRequestData struct {
	Method       string                 `json:"method"`
	Args         map[string]interface{} `json:"args"`
	SessionToken string                 `json:"sessionToken"`
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

// Is token present in Redis database
func isSessionTokenPresent(rdb *redis.Client, token string) (error, bool) {
	err := rdb.Get("sessionToken:" + token).Err()
	if err != nil && err != redis.Nil {
		return err, false
	}

	return nil, err != redis.Nil
}

// Check for existing cooldown information in Redis database.
// If there is none, insert new. Should be atomic.
func testAndUpdateSessionCooldown(rdb *redis.Client, token string) (error, bool) {
	// Unfortunately, GETSET command has no expiration time argument. Also there is no test-and-set command.
	// So we do this:
	// x = GETSET token, expireTime
	// if x is present and x > now:
	//   SET token x
	// Also add expire time to avoid outdated records.
	key := "cooldown:" + token
	currentTime := time.Now().Unix()
	expireTime := currentTime + CooldownSeconds

	oldValStr, err := rdb.GetSet(key, expireTime).Result()
	if err != nil && err != redis.Nil {
		return err, false
	}

	if err == nil {
		if oldVal, err := strconv.ParseInt(oldValStr, 10, 64); err == nil {
			if oldVal > currentTime {
				if err := rdb.Set(key, oldValStr, CooldownSeconds*time.Second).Err(); err != nil {
					return err, false
				}
				return nil, true
			}
		}
	}

	// Set expire time to avoid outdated records.
	if err := rdb.Expire(key, CooldownSeconds*time.Second).Err(); err != nil {
		return err, false
	}

	return nil, false
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

// Handle setPixelColor method.
//
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
func handleSetPixelColor(
	wsMessage *WebSocketRequestData,
	mt int,
	c *websocket.Conn,
	rdb *redis.Client,
	matrix *[CanvasRows][CanvasCols]string,
	allConnections map[*websocket.Conn]struct{},
) bool {
	pixel, err := argsToPixelInfo(wsMessage.Args)
	if err != nil {
		// Problems with user data. Just ignore.
		logError("unmarshal (data)", err)
		return true
	}

	err, tokenPresent := isSessionTokenPresent(rdb, wsMessage.SessionToken)
	if err != nil {
		logError("redis check token", err)
		return false
	}
	if !tokenPresent {
		// Session does not exist. Ignore request.
		return true
	}

	err, hasOldCooldown := testAndUpdateSessionCooldown(rdb, wsMessage.SessionToken)
	if err != nil {
		logError("update redis cooldown", err)
	}
	if hasOldCooldown {
		// Cooldown time does not expire yet. Maybe cheating. Ignore request.
		return true
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
		return true
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
		return false
	}

	return true
}

// Handle connectMe method
//
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
func handleConnectMe(
	wsMessage *WebSocketRequestData,
	mt int,
	c *websocket.Conn,
	rdb *redis.Client,
	matrix *[CanvasRows][CanvasCols]string,
	allConnections map[*websocket.Conn]struct{},
) bool {
	log.Printf("connectMe()\n")

	_, ok := allConnections[c]
	if !ok {
		allConnections[c] = struct{}{}
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

	// Add session token to Redis.
	// It's workaround. Token should be added there when user is logging in.
	// TODO: Add auth and remove it.
	err := rdb.Set("sessionToken:"+wsMessage.SessionToken, "", 0).Err()
	if err != nil {
		logError("write session token", err)
		return false
	}

	wsResponse := WebSocketResponseData{
		Kind: "allPixelsColors",
		Data: pixelsData,
	}
	response, err := json.Marshal(&wsResponse)
	if err != nil {
		logError("marshal", err)
		return true
	}

	err = c.WriteMessage(mt, response)
	if err != nil {
		if isWsClosedOk(err) {
			delete(allConnections, c)
		} else {
			logError("read", err)
		}
		return false
	}

	cooldownStr, err := rdb.Get("cooldown:" + wsMessage.SessionToken).Result()
	if err != nil && err != redis.Nil {
		logError("redis read cooldown", err)
		return false
	}
	if err == nil {
		cooldown, err := strconv.ParseInt(cooldownStr, 10, 64)
		if err != nil {
			logError("redis read cooldown (invalid value)", err)
			return false
		}
		currentTime := time.Now().Unix()

		wsResponse := WebSocketResponseData{
			Kind: "cooldownInfo",
			Data: cooldown - currentTime,
		}
		response, err := json.Marshal(&wsResponse)
		if err != nil {
			logError("marshal", err)
			return true
		}

		err = c.WriteMessage(mt, response)
		if err != nil {
			if isWsClosedOk(err) {
				delete(allConnections, c)
			} else {
				logError("read", err)
			}
			return false
		}
	}

	return true
}

// Handle client requests.
func serve(
	w http.ResponseWriter,
	r *http.Request,
	rdb *redis.Client,
	matrix *[CanvasRows][CanvasCols]string,
	allConnections map[*websocket.Conn]struct{},
) {
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

		ok := true
		switch wsMessage.Method {
		case "setPixelColor":
			ok = handleSetPixelColor(&wsMessage, mt, c, rdb, matrix, allConnections)
		case "connectMe":
			ok = handleConnectMe(&wsMessage, mt, c, rdb, matrix, allConnections)
		default:
			logError("unsupported method", nil)
		}

		if !ok {
			return
		}
	}
}

func main() {
	// List of all connections.
	// We store all websocket connections with active users. When someone has changed pixel color we iterate over
	// `allConnections' and notify each user about changes.
	allConnections := make(map[*websocket.Conn]struct{})

	// Allocate canvas matrix. Items are colors.
	matrix := [CanvasRows][CanvasCols]string{}
	for y := 0; y < CanvasRows; y++ {
		for x := 0; x < CanvasCols; x++ {
			if (x+y)%2 == 0 {
				matrix[y][x] = "gray"
			} else {
				matrix[y][x] = "white"
			}
		}
	}

	// Redis connection. Redis stores session info and cooldowns.
	rdb := redis.NewClient(&redis.Options{
		Addr:     RedisAddr,
		Password: RedisPassword,
		DB:       RedisDatabase,
	})
	err := rdb.Ping().Err()
	if err != nil {
		log.Fatal("cannot connect to redis server", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, rdb, &matrix, allConnections)
	})
	log.Fatal(http.ListenAndServe(ListenAddr, nil))
}
