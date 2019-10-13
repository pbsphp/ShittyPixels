#!/usr/bin/env python

# ShittyPixels
# Copyright © 2019  Pbsphp

# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.

# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.

# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

import asyncio
import websockets
import json


# Also hardcoded in index.html
CANVAS_ROWS = 50
CANVAS_COLS = 100


matrix = []

for y in range(CANVAS_ROWS):
    row = []
    for x in range(CANVAS_COLS):
        row.append("gray" if (x + y) % 2 == 0 else "white")
    matrix.append(row)


all_connections = set()


async def serve(websocket, path):
    while True:
        try:
            raw_request = await websocket.recv()
        except websockets.exceptions.ConnectionClosedOK:
            print("Disconnected")

            all_connections.discard(websocket)

            break
        else:
            print(f"> {raw_request}")

            request = json.loads(raw_request)

            if request["method"] == "setPixelColor":
                args = request["args"]
                x = args["x"]
                y = args["y"]

                matrix[y][x] = args["color"]

                response = {
                    "kind": "pixelColor",
                    "data": {
                        "x": x,
                        "y": y,
                        "color": args["color"],
                    },
                }
                raw_response = json.dumps(response)

                for conn in all_connections:
                    await conn.send(raw_response)

                print(f">> {raw_response}")

            elif request["method"] == "connectMe":
                if websocket not in all_connections:
                    all_connections.add(websocket)
                    print("Connected")

                response_data = []
                for y, row in enumerate(matrix, start=0):
                    for x, color in enumerate(row, start=0):
                        response_data.append({
                            "x": x,
                            "y": y,
                            "color": color,
                        })

                response = {
                    "kind": "allPixelsColors",
                    "data": response_data,
                }
                raw_response = json.dumps(response)
                await websocket.send(raw_response)

                print(f">> {raw_response[:70]}...")

            else:
                raise RuntimeError("TODO")


start_server = websockets.serve(serve, "localhost", 8765)

asyncio.get_event_loop().run_until_complete(start_server)
asyncio.get_event_loop().run_forever()
