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
	"flag"
	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
	"github.com/pbsphp/ShittyPixels/common"
	"golang.org/x/image/colornames"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"os"
	"regexp"
)

// Color of pixel
type Color uint8

func (c *Color) UnmarshalJSON(b []byte) error {
	var val int
	err := json.Unmarshal(b, &val)
	if err != nil {
		return err
	}
	*c = Color(val)

	return nil
}

func (c Color) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(c))
}

// Print error message with [ ERROR ] prefix and description.
func logError(description string, err error) {
	log.Println("[ ERROR ]: ", description, err)
}

type Matrix struct {
	// Array of colors. Only pixels belonging to this instance.
	// So len(Data) < Width * Height!
	Data []Color
	// Total width of canvas.
	Width int
	// Total height of canvas.
	Height int

	instanceNumber int
	totalInstances int
}

func NewMatrix(width, height, instanceNumber, totalInstances int) Matrix {
	instanceWidth := (width + totalInstances - 1) / totalInstances
	return Matrix{
		Data:           make([]Color, instanceWidth*height),
		Width:          width,
		Height:         height,
		instanceNumber: instanceNumber,
		totalInstances: totalInstances,
	}
}

func (m *Matrix) Get(x, y int) (Color, bool) {
	if x%m.totalInstances != m.instanceNumber {
		return 0, false
	}

	instanceX := x / m.totalInstances
	instanceWidth := (m.Width + m.totalInstances - 1) / m.totalInstances

	return m.Data[y*instanceWidth+instanceX], true
}

func (m *Matrix) Set(x, y int, val Color) bool {
	if x%m.totalInstances != m.instanceNumber {
		return false
	}

	instanceX := x / m.totalInstances
	instanceWidth := (m.Width + m.totalInstances - 1) / m.totalInstances

	m.Data[y*instanceWidth+instanceX] = val
	return true
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
	X     int   `json:"x"`
	Y     int   `json:"y"`
	Color Color `json:"color"`
}

// Is token present in Redis database
func isSessionTokenPresent(rdb *redis.Client, token string) (error, bool) {
	err := rdb.Get("sessionToken:" + token).Err()
	if err != nil && err != redis.Nil {
		return err, false
	}

	return nil, err != redis.Nil
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
	colorNumber, ok := rawColor.(float64)
	if !ok {
		return nil, errors.New("expected 'color':Number key")
	}

	return &PixelInfo{
		X:     int(x),
		Y:     int(y),
		Color: Color(colorNumber),
	}, nil
}

// Is error caused due to closed WebSocket connection.
// If websocket connection is closed, it's OK. Do not log it.
func isWsClosedOk(err error) bool {
	return websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure)
}

// Read PNG image and draw it on the matrix using closest available color.
// Panic on failure.
func MustDrawInitialImage(
	path string,
	matrix *Matrix,
	palette []string,
	instanceNumber int,
	totalInstances int,
) {
	checkError := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	pow2 := func(x float32) float32 {
		return x * x
	}

	// Transform palette with color names to RGBA
	paletteRGBA := make([]color.RGBA, len(palette))
	for i, colorName := range palette {
		paletteRGBA[i] = colornames.Map[colorName]
	}

	// Return index of palette color closest to given RGB color.
	// Distance formula is: (0.3(R1 - R2))^2 + (0.59(G1 - G2))^2 + (0.11(B1 - B2))^2.
	// See https://stackoverflow.com/a/1847112.
	getClosestColor := func(first color.RGBA) Color {
		var minDistance float32
		var minDistanceColorIndex int
		for i, second := range paletteRGBA {
			dist := pow2((float32(first.R)-float32(second.R))*0.3) +
				pow2((float32(first.G)-float32(second.G))*0.59) +
				pow2((float32(first.B)-float32(second.B))*0.11)

			if i == 0 || dist < minDistance {
				minDistance = dist
				minDistanceColorIndex = i
			}
		}
		return Color(minDistanceColorIndex)
	}

	f, err := os.Open(path)
	checkError(err)
	defer func() {
		checkError(f.Close())
	}()

	img, err := png.Decode(f)
	checkError(err)

	imgWidth := img.Bounds().Max.X - img.Bounds().Min.X
	imgHeight := img.Bounds().Max.Y - img.Bounds().Min.Y
	canvasWidth := matrix.Width
	canvasHeight := matrix.Height

	repeatX := (canvasWidth + imgWidth - 1) / imgWidth
	repeatY := (canvasHeight + imgHeight - 1) / imgHeight

	// TODO: Rewrite this loops.
	for rY := 0; rY < repeatY; rY++ {
		for rX := 0; rX < repeatX; rX++ {
			for y := 0; y < imgHeight; y++ {
				imgY := y + img.Bounds().Min.Y
				matrixY := rY*imgHeight + y
				for x := instanceNumber; x < imgWidth; x += totalInstances {
					imgX := x + img.Bounds().Min.X
					matrixX := rX*imgWidth + x

					if imgX < img.Bounds().Max.X &&
						imgY < img.Bounds().Max.Y &&
						matrixX < canvasWidth &&
						matrixY < canvasHeight {
						imgColor := color.RGBAModel.Convert(img.At(imgX, imgY)).(color.RGBA)
						ok := matrix.Set(matrixX, matrixY, getClosestColor(imgColor))
						if !ok {
							// Expected to be unreachable.
							panic("initial picture drawing failed (unreachable code)")
						}
					}
				}
			}
		}
	}
}

type WebSocketHandler struct {
	rdb            *redis.Client
	appConfig      *common.AppConfig
	upgraderConfig websocket.Upgrader

	allConnections map[*websocket.Conn]struct{}
	matrix         *Matrix

	instanceNumber int
	totalInstances int
}

func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := h.upgraderConfig.Upgrade(w, r, nil)
	if err != nil {
		logError("upgrade", err)
		return
	}
	defer func() {
		if err := c.Close(); err != nil {
			logError("close connection", err)
		}
	}()

	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			if !isWsClosedOk(err) {
				logError("read", err)
			}
			delete(h.allConnections, c)
			return
		}

		wsMessage := WebSocketRequestData{}
		err = json.Unmarshal(message, &wsMessage)
		if err != nil {
			logError("unmarshal", err)
			continue
		}

		// Check that user is authenticated (session has Login)
		// Check that user logged in (has active session with login)
		session, err := common.GetSessionBySessionId(h.rdb, wsMessage.SessionToken)
		if err != nil {
			logError("get session info", err)
			continue
		}
		if session == nil || session.Login == "" {
			// Cheating? Ignore request.
			continue
		}

		ok := true
		switch wsMessage.Method {
		case "setPixelColor":
			ok = h.handleSetPixelColor(&wsMessage, mt, c)
		case "connectMe":
			ok = h.handleConnectMe(&wsMessage, mt, c)
		default:
			logError("unsupported method", nil)
		}

		if !ok {
			return
		}
	}

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
func (h *WebSocketHandler) handleSetPixelColor(wsMessage *WebSocketRequestData, mt int, c *websocket.Conn) bool {
	pixel, err := argsToPixelInfo(wsMessage.Args)
	if err != nil {
		// Problems with user data. Just ignore.
		logError("unmarshal (data)", err)
		return true
	}

	err, hasOldCooldown := common.TestAndUpdateSessionCooldown(h.rdb, h.appConfig, wsMessage.SessionToken)
	if err != nil {
		logError("update redis cooldown", err)
		return true
	}
	if hasOldCooldown {
		// Cooldown time does not expire yet. Maybe cheating. Ignore request.
		return true
	}

	ok := h.matrix.Set(pixel.X, pixel.Y, pixel.Color)
	if !ok {
		// This pixel is managed by other worker.
		// Ignore request.
		return true
	}

	log.Printf("setPixelColor(x=%d, y=%d, color(code)=%d)\n", pixel.X, pixel.Y, pixel.Color)

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
	for conn := range h.allConnections {
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
		delete(h.allConnections, conn)

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
//         pixelColor,
//         anotherPixelColor,
// 		   ...
//     ]
// }
func (h *WebSocketHandler) handleConnectMe(wsMessage *WebSocketRequestData, mt int, c *websocket.Conn) bool {
	log.Printf("connectMe()\n")

	_, ok := h.allConnections[c]
	if !ok {
		h.allConnections[c] = struct{}{}
	}

	wsResponse := WebSocketResponseData{
		Kind: "allPixelsColors",
		Data: struct {
			ColorCodes []Color `json:"colorCodes"`
			Offset     int     `json:"offset"`
			EachNth    int     `json:"eachNth"`
		}{
			ColorCodes: h.matrix.Data,
			Offset:     h.instanceNumber,
			EachNth:    h.totalInstances,
		},
	}
	response, err := json.Marshal(&wsResponse)
	if err != nil {
		logError("marshal", err)
		return true
	}

	err = c.WriteMessage(mt, response)
	if err != nil {
		if isWsClosedOk(err) {
			delete(h.allConnections, c)
		} else {
			logError("read", err)
		}
		return false
	}

	// Also send cooldown info (if present)
	cooldown, err := common.GetSessionCooldownBySessionId(h.rdb, wsMessage.SessionToken)
	if err != nil {
		logError("redis read cooldown", err)
		return false
	}
	if cooldown > 0 {
		wsResponse := WebSocketResponseData{
			Kind: "cooldownInfo",
			Data: cooldown,
		}
		response, err := json.Marshal(&wsResponse)
		if err != nil {
			logError("marshal", err)
			return true
		}

		err = c.WriteMessage(mt, response)
		if err != nil {
			if isWsClosedOk(err) {
				delete(h.allConnections, c)
			} else {
				logError("read", err)
			}
			return false
		}
	}

	return true
}

func main() {
	instanceNumberFlag := flag.Int("n", -1, "instance number")
	listenAddressFlag := flag.String("listen", "", "address to listen")
	flag.Parse()

	appConfig := common.MustReadAppConfig("config.json")

	instanceNumber := *instanceNumberFlag
	totalInstances := len(appConfig.WebSocketAppAddresses)
	if instanceNumber < 0 || instanceNumber > totalInstances {
		panic("provide -n=x argument (0 <= x < len(WebSocketAppAddresses))")
	}
	listenAddress := *listenAddressFlag
	if listenAddress == "" {
		panic("provide -listen=[host]:port")
	}

	// List of all connections.
	// We store all websocket connections with active users. When someone has changed pixel color we iterate over
	// `allConnections' and notify each user about changes.
	allConnections := make(map[*websocket.Conn]struct{})

	// Allocate canvas matrix. Items are colors.
	matrix := NewMatrix(appConfig.CanvasCols, appConfig.CanvasRows, instanceNumber, totalInstances)

	MustDrawInitialImage(
		appConfig.InitialImage,
		&matrix,
		appConfig.PaletteColors,
		instanceNumber,
		totalInstances,
	)

	// Redis connection. Redis stores session info and cooldowns.
	rdb := redis.NewClient(&redis.Options{
		Addr:     appConfig.RedisAddress,
		Password: appConfig.RedisPassword,
		DB:       appConfig.RedisDatabase,
	})
	err := rdb.Ping().Err()
	if err != nil {
		log.Fatal("cannot connect to redis server", err)
	}

	allowedOriginPattern := regexp.MustCompile(appConfig.AllowedOrigins)
	upgraderConfig := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header["Origin"]
			if len(origin) == 0 {
				return true
			}
			return allowedOriginPattern.MatchString(origin[0])
		},
	}

	handler := WebSocketHandler{
		rdb:            rdb,
		appConfig:      appConfig,
		upgraderConfig: upgraderConfig,

		allConnections: allConnections,
		matrix:         &matrix,

		instanceNumber: instanceNumber,
		totalInstances: totalInstances,
	}

	http.Handle("/", &handler)
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
