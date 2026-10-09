#!/usr/bin/env python3
"""Генерирует icon.png и icon.ico (несколько размеров) без внешних библиотек."""
import struct, zlib, os, sys

OUT = sys.argv[1] if len(sys.argv) > 1 else "."

def lerp(a, b, t):
    return a + (b - a) * t

def in_rounded_rect(x, y, r):
    # x,y in [0,1]; скруглённый квадрат со всех сторон
    if x < r:
        dx = r - x
    elif x > 1 - r:
        dx = x - (1 - r)
    else:
        dx = 0
    if y < r:
        dy = r - y
    elif y > 1 - r:
        dy = y - (1 - r)
    else:
        dy = 0
    if dx == 0 or dy == 0:
        return 0 <= x <= 1 and 0 <= y <= 1
    return dx * dx + dy * dy <= r * r

def in_rect(x, y, x0, y0, x1, y1):
    return x0 <= x <= x1 and y0 <= y <= y1

def in_tri(px, py, ax, ay, bx, by, cx, cy):
    def sign(x1, y1, x2, y2, x3, y3):
        return (x1 - x3) * (y2 - y3) - (x2 - x3) * (y1 - y3)
    d1 = sign(px, py, ax, ay, bx, by)
    d2 = sign(px, py, bx, by, cx, cy)
    d3 = sign(px, py, cx, cy, ax, ay)
    neg = (d1 < 0) or (d2 < 0) or (d3 < 0)
    pos = (d1 > 0) or (d2 > 0) or (d3 > 0)
    return not (neg and pos)

# Палитра
C1 = (0x2a, 0x78, 0xd6)   # синий
C2 = (0x39, 0x87, 0xe5)   # светло-синий
WHITE = (255, 255, 255)

def sample(x, y):
    """Возвращает RGBA в точке (x,y) в [0,1]."""
    if not in_rounded_rect(x, y, 0.22):
        return (0, 0, 0, 0)
    t = (x + y) / 2.0
    r = int(lerp(C1[0], C2[0], t))
    g = int(lerp(C1[1], C2[1], t))
    b = int(lerp(C1[2], C2[2], t))

    # Верхняя стрелка: вправо
    top = in_rect(x, y, 0.28, 0.345, 0.64, 0.415) or \
          in_tri(x, y, 0.60, 0.295, 0.60, 0.465, 0.76, 0.38)
    # Нижняя стрелка: влево
    bot = in_rect(x, y, 0.36, 0.585, 0.72, 0.655) or \
          in_tri(x, y, 0.40, 0.535, 0.40, 0.705, 0.24, 0.62)

    if top or bot:
        return (WHITE[0], WHITE[1], WHITE[2], 255)
    return (r, g, b, 255)

def render(size, ss=4):
    """Рендер с супер-сэмплингом ss×ss."""
    px = bytearray()
    n = size * ss
    # аккумулируем
    acc = [[[0, 0, 0, 0] for _ in range(size)] for _ in range(size)]
    for j in range(n):
        yy = (j + 0.5) / n
        for i in range(n):
            xx = (i + 0.5) / n
            r, g, b, a = sample(xx, yy)
            oi, oj = i // ss, j // ss
            cell = acc[oj][oi]
            cell[0] += r; cell[1] += g; cell[2] += b; cell[3] += a
    inv = 1.0 / (ss * ss)
    raw = bytearray()
    for j in range(size):
        raw.append(0)  # filter type 0
        for i in range(size):
            cell = acc[j][i]
            raw += bytes((int(cell[0] * inv), int(cell[1] * inv), int(cell[2] * inv), int(cell[3] * inv)))
    return bytes(raw)

def png_bytes(size, raw):
    def chunk(typ, data):
        c = struct.pack(">I", len(data)) + typ + data
        c += struct.pack(">I", zlib.crc32(typ + data) & 0xffffffff)
        return c
    sig = b"\x89PNG\r\n\x1a\n"
    ihdr = struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)
    idat = zlib.compress(raw, 9)
    return sig + chunk(b"IHDR", ihdr) + chunk(b"IDAT", idat) + chunk(b"IEND", b"")

def make_png(size, path):
    raw = render(size)
    open(path, "wb").write(png_bytes(size, raw))

def make_ico(sizes, path):
    entries = []
    for sz in sizes:
        raw = render(sz)
        entries.append((sz, png_bytes(sz, raw)))
    # ICO header
    out = struct.pack("<HHH", 0, 1, len(entries))
    offset = 6 + 16 * len(entries)
    dir_entries = b""
    data_blob = b""
    for sz, data in entries:
        w = 0 if sz >= 256 else sz
        h = 0 if sz >= 256 else sz
        dir_entries += struct.pack("<BBBBHHII", w, h, 0, 0, 1, 32, len(data), offset)
        offset += len(data)
        data_blob += data
    open(path, "wb").write(out + dir_entries + data_blob)

if __name__ == "__main__":
    make_png(256, os.path.join(OUT, "icon.png"))
    make_ico([16, 32, 48, 64, 128, 256], os.path.join(OUT, "icon.ico"))
    print("icon.png и icon.ico созданы в", OUT)
