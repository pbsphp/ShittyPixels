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
        canvas.width = config["CanvasRows"] * PIXEL_SIZE;
        canvas.height = config["CanvasCols"] * PIXEL_SIZE;

        this.sock = new WebSocket(config["WebSocketAppAddr"]);
        this.sock.onmessage = this.handleMessage;
        this.sock.onopen = this.connect;

        this.paletteWidget = paletteWidget;
        this.timerWidget = timerWidget;

        this.sessionToken = sessionToken;
        this.config = config;
    }

    connect() {
        this.sock.send(
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

            this.sock.send(
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
        const totalRows = this.config["CanvasRows"];
        const totalCols = this.config["CanvasCols"];

        for (let y = 0; y < totalRows; ++y) {
            for (let x = 0; x < totalCols; ++x) {
                const colorCode = data[y * totalCols + x];
                const colorName = colorsTable[colorCode];
                this.canvasWrapper.setPixelColor(x, y, colorName);
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
        this.handleColorChoose = this.handleColorChoose.bind(this);

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
            cell.onclick = this.handleColorChoose;
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
