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

const WEB_SOCKET_ADDR = "ws://localhost:8765/";

// Stub. Will be unique for each tab.
// TODO: users and auth.
const SESSION_TOKEN = "token_" + Math.random();

const COOLDOWN_SECONDS = 5;


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
    constructor(canvas, paletWidget, timerWidget) {
        this.connect = this.connect.bind(this);
        this.handleMessage = this.handleMessage.bind(this);
        this.handleCanvasClick = this.handleCanvasClick.bind(this);
        this.handlePixelColorMessage = this.handlePixelColorMessage.bind(this);
        this.handleAllPixelsColorsMessage = this.handleAllPixelsColorsMessage.bind(this);

        this.canvasWrapper = new CanvasWrapper(canvas);
        canvas.onclick = this.handleCanvasClick;

        this.sock = new WebSocket(WEB_SOCKET_ADDR);
        this.sock.onmessage = this.handleMessage;
        this.sock.onopen = this.connect;

        this.paletWidget = paletWidget;
        this.timerWidget = timerWidget;
    }

    connect() {
        this.sock.send(
            JSON.stringify({
                method: "connectMe",
                sessionToken: SESSION_TOKEN,
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
        if (this.timerWidget.cooldownExpiry === null) {
            const canvas = this.canvasWrapper.canvas;
            const rect = canvas.getBoundingClientRect();
            const realX = evt.clientX - rect.left;
            const realY = evt.clientY - rect.top;
            const x = Math.floor(realX / PIXEL_SIZE);
            const y = Math.floor(realY / PIXEL_SIZE);

            this.sock.send(
                JSON.stringify({
                    method: "setPixelColor",
                    sessionToken: SESSION_TOKEN,
                    args: {
                        x: x,
                        y: y,
                        color: this.paletWidget.selectedColor,
                    },
                })
            );

            this.timerWidget.countDown(COOLDOWN_SECONDS);
        }
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


class PaletWidget {
    constructor(tableDomElement, colorsList) {
        this.fillPaletTable = this.fillPaletTable.bind(this);
        this.selectCell = this.selectCell.bind(this);
        this.handleColorChoose = this.handleColorChoose.bind(this);

        this.tableDomElement = tableDomElement;
        this.colorsList = colorsList;

        this.selectedColor = null;
    }

    fillPaletTable() {
        const paletRow = this.tableDomElement.insertRow(0);
        for (let color of paletConfig) {
            const cell = paletRow.insertCell(-1);
            cell.classList.add("palet-cell");
            cell.style.backgroundColor = color;
            cell.dataset.color = color;
            cell.onclick = this.handleColorChoose;
        }

        this.selectCell(paletRow.cells[0]);
    }

    selectCell(selectedCell) {
        this.selectedColor = selectedCell.dataset.color;

        const oldCells = this.tableDomElement.getElementsByClassName(
            "pallet-cell-selected");
        for (let cell of oldCells) {
            cell.classList.remove("pallet-cell-selected");
        }

        selectedCell.classList.add("pallet-cell-selected");
    }

    handleColorChoose(evt) {
        this.selectCell(evt.srcElement);
    }
}


class TimerWidget {
    constructor(domElement) {
        this.updateValue = this.updateValue.bind(this);
        this.countDown = this.countDown.bind(this);

        this.domElement = domElement;
        this.cooldownExpiry = null;

        // Simple ascii-animation.
        this.progressBarStates = [
            "/", "−", "\\", "|",
        ];
        this.progressBarState = 0;

        this.intervalObj = null;
    }

    updateValue(sec) {
        const progressBarIcon = this.progressBarStates[this.progressBarState];
        this.progressBarState = (
            (this.progressBarState + 1) % this.progressBarStates.length);
        this.domElement.innerHTML = (
            "" + sec + "&nbsp&nbsp&nbsp" + progressBarIcon);
    }

    countDown(seconds) {
        const dateNow = () => Math.floor((new Date()).getTime() / 1000);
        this.cooldownExpiry = dateNow() + seconds;
        this.intervalObj = setInterval(() => {
            const secondsToWait = this.cooldownExpiry - dateNow();
            if (secondsToWait > 0) {
                this.updateValue(secondsToWait);
            } else {
                this.domElement.innerHTML = "";
                this.cooldownExpiry = null;
                clearInterval(this.intervalObj);
                this.intervalObj = null;
            }
        }, 100);
    }
}
