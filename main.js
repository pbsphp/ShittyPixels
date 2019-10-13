// ShittyPixels
// Copyright © 2019  Pbsphp

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.


const PIXEL_SIZE = 10;
const CANVAS_ROWS = 50;
const CANVAS_COLS = 100;


class CanvasWrapper {
    constructor(canvas) {
        this.canvas = canvas;
        this.ctx = canvas.getContext("2d");
    }

    setPixelColor(x, y, color) {
        this.ctx.fillStyle = color;
        this.ctx.fillRect(
            x * PIXEL_SIZE,
            y * PIXEL_SIZE,
            PIXEL_SIZE,
            PIXEL_SIZE,
        );
    }
}


class Controller {
    constructor(canvas) {
        this.connect = this.connect.bind(this);
        this.handleMessage = this.handleMessage.bind(this);
        this.handleCanvasClick = this.handleCanvasClick.bind(this);
        this.handlePixelColorMessage = this.handlePixelColorMessage.bind(this);
        this.handleAllPixelsColorsMessage = this.handleAllPixelsColorsMessage.bind(this);

        this.canvasWrapper = new CanvasWrapper(canvas);
        canvas.onclick = this.handleCanvasClick;

        this.sock = new WebSocket("ws://localhost:8765/");
        this.sock.onmessage = this.handleMessage;
        this.sock.onopen = this.connect;
    }

    connect() {
        this.sock.send(
            JSON.stringify({
                method: "connectMe",
            })
        );
    }

    handleMessage(evt) {
        const message = JSON.parse(evt.data);
        switch (message.kind) {
        case "pixelColor":
            this.handlePixelColorMessage(message.data);
            break;
        case "allPixelsColors":
            this.handleAllPixelsColorsMessage(message.data);
            break;

        default:
            alert("FAIL (fixme)");
        }
    }

    handleCanvasClick(evt) {
        const canvas = this.canvasWrapper.canvas;
        const rect = canvas.getBoundingClientRect();
        const realX = evt.clientX - rect.left;
        const realY = evt.clientY - rect.top;
        const x = Math.floor(realX / PIXEL_SIZE);
        const y = Math.floor(realY / PIXEL_SIZE);

        this.sock.send(
            JSON.stringify({
                method: "setPixelColor",
                args: {
                    x: x,
                    y: y,
                    color: "black"
                },
            })
        );
    }

    handlePixelColorMessage(data) {
        this.canvasWrapper.setPixelColor(
            data.x, data.y, data.color);
    }

    handleAllPixelsColorsMessage(data) {
        for (let datum of data) {
            this.handlePixelColorMessage(datum);
        }
    }
}
