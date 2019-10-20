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
    constructor(config, sessionToken, canvas, paletteWidget, timerWidget) {
        this.connect = this.connect.bind(this);
        this.handleMessage = this.handleMessage.bind(this);
        this.handleCanvasClick = this.handleCanvasClick.bind(this);
        this.handlePixelColorMessage = this.handlePixelColorMessage.bind(this);
        this.handleAllPixelsColorsMessage = this.handleAllPixelsColorsMessage.bind(this);
        this.handleCooldownInfoMessage = this.handleCooldownInfoMessage.bind(this);

        this.canvasWrapper = new CanvasWrapper(canvas);
        canvas.onclick = this.handleCanvasClick;
        canvas.width = config["CanvasCols"] * PIXEL_SIZE;
        canvas.height = config["CanvasRows"] * PIXEL_SIZE;

        this.connections = [];
        for (let addr of config["WebSocketAppAddresses"]) {
            const conn = new WebSocket(addr);
            conn.onmessage = this.handleMessage;
            conn.onopen = () => this.connect(conn);
            this.connections.push(conn);
        }

        this.paletteWidget = paletteWidget;
        this.timerWidget = timerWidget;

        this.sessionToken = sessionToken;
        this.config = config;
    }

    connect(conn) {
        conn.send(
            JSON.stringify({
                method: "connectMe",
                sessionToken: this.sessionToken,
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
        case "cooldownInfo":
            this.handleCooldownInfoMessage(message.data);
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

            const connIndex = x % this.connections.length;
            const conn = this.connections[connIndex];

            conn.send(
                JSON.stringify({
                    method: "setPixelColor",
                    sessionToken: this.sessionToken,
                    args: {
                        x: x,
                        y: y,
                        color: this.paletteWidget.selectedColorCode,
                    },
                })
            );

            this.timerWidget.countDown(this.config["CooldownSeconds"]);
        }
    }

    handlePixelColorMessage(data) {
        const colorName = this.paletteWidget.colorsList[data.color];
        this.canvasWrapper.setPixelColor(
            data.x, data.y, colorName);
    }

    handleAllPixelsColorsMessage(data) {
        const colorsTable = this.paletteWidget.colorsList;

        const totalWidth = this.config["CanvasCols"];
        const totalHeight = this.config["CanvasRows"];

        const matrix = data["colorCodes"];
        const offset = data["offset"];
        const eachNth = data["eachNth"];

        const instanceWidth = Math.ceil(totalWidth / eachNth);

        for (let y = 0; y < totalHeight; ++y) {
            for (let instanceX = 0; instanceX < instanceWidth; ++instanceX) {
                const x = instanceX * eachNth + offset;
                if (x < totalWidth) {
                    const colorCode = matrix[y * instanceWidth + instanceX];
                    const colorName = colorsTable[colorCode];
                    this.canvasWrapper.setPixelColor(x, y, colorName);
                }
            }
        }
    }

    handleCooldownInfoMessage(data) {
        this.timerWidget.countDown(data);
    }
}


class PaletteWidget {
    constructor(tableDomElement, colorsList) {
        this.fillPaletteTable = this.fillPaletteTable.bind(this);
        this.selectCell = this.selectCell.bind(this);

        this.tableDomElement = tableDomElement;
        this.colorsList = colorsList;

        // Actually this is index of color from colorsList
        this.selectedColorCode = null;
    }

    fillPaletteTable() {
        const paletteRow = this.tableDomElement.insertRow(0);
        for (let i = 0; i < this.colorsList.length; ++i) {
            const cell = paletteRow.insertCell(-1);
            cell.classList.add("palette-cell");
            cell.style.backgroundColor = this.colorsList[i];
            cell.dataset.color = i.toString();
            cell.onclick = () => this.selectCell(cell);
        }

        this.selectCell(paletteRow.cells[0]);
    }

    selectCell(selectedCell) {
        this.selectedColorCode = parseInt(selectedCell.dataset.color);

        const oldCells = this.tableDomElement.getElementsByClassName(
            "pallet-cell-selected");
        for (let cell of oldCells) {
            cell.classList.remove("pallet-cell-selected");
        }

        selectedCell.classList.add("pallet-cell-selected");
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
        if (this.intervalObj !== null) {
            clearInterval(this.intervalObj);
            this.intervalObj = null;
        }

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
