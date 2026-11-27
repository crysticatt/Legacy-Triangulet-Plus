#!/usr/bin/env python3
"""Writes rsrc_windows_amd64.syso: app icon, manifest (DPI aware, visual styles) and version info.
usage: genres.py <version>   (reads triangulet-logo.png, writes rsrc_windows_amd64.syso)"""
import struct, sys, io
from PIL import Image

ver = sys.argv[1] if len(sys.argv) > 1 else "31"
logo = Image.open("triangulet-logo.png").convert("RGBA")
sizes = [256, 128, 64, 48, 32, 24, 16]
pngs = []
for s in sizes:
    b = io.BytesIO(); logo.resize((s, s), Image.LANCZOS).save(b, "PNG"); pngs.append(b.getvalue())
logo.resize((256, 256), Image.LANCZOS).save("appicon.png")

manifest = f'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
 <assemblyIdentity type="win32" name="TrianguletOpener" version="{ver}.0.0.0" processorArchitecture="*"/>
 <description>Triangulet Plus</description>
 <dependency><dependentAssembly><assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/></dependentAssembly></dependency>
 <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3"><security><requestedPrivileges><requestedExecutionLevel level="asInvoker" uiAccess="false"/></requestedPrivileges></security></trustInfo>
 <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1"><application>
  <supportedOS Id="{{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}}"/>
 </application></compatibility>
 <application xmlns="urn:schemas-microsoft-com:asm.v3"><windowsSettings>
  <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
  <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2,PerMonitor</dpiAwareness>
 </windowsSettings></application>
</assembly>'''.encode()

def u16(s): return s.encode("utf-16-le") + b"\0\0"
def pad4(b): return b + b"\0" * ((-len(b)) % 4)

def vnode(key, value=b"", wtype=1, children=b"", vlen=None):
    k = u16(key)
    head = struct.pack("<HHH", 0, len(value) if vlen is None else vlen, wtype)
    body = pad4(head + k) + value
    body = pad4(body) + children
    return struct.pack("<H", len(body) + 0) [:0] or (struct.pack("<H", len(body)) + body[2:])

def node(key, value=b"", wtype=1, children=b"", vlen=None):
    k = u16(key)
    hdr = struct.pack("<HH", vlen if vlen is not None else len(value), wtype)
    body = pad4(hdr + k) if True else b""
    # wLength(2) precedes hdr; alignment is relative to start of node
    start = struct.pack("<H", 0) + hdr + k
    start = pad4(start) + value
    start = pad4(start) + children
    return struct.pack("<H", len(start)) + start[2:]

v = [int(x) for x in (ver.split(".") + ["0", "0", "0"])[:4]]
fixed = struct.pack("<IIIIIIIIIIIII", 0xFEEF04BD, 0x10000, (v[0] << 16) | v[1], (v[2] << 16) | v[3], (v[0] << 16) | v[1], (v[2] << 16) | v[3],
                    0x3F, 0, 0x40004, 1, 0, 0, 0)
strs = {"CompanyName": "Triangulet Plus", "FileDescription": "Triangulet Plus - background capsule opener",
        "FileVersion": ver + ".0.0.0", "InternalName": "TrianguletOpener", "OriginalFilename": "TrianguletOpener.exe",
        "ProductName": "Triangulet Plus", "ProductVersion": ver + ".0.0.0", "LegalCopyright": "Fan-made tool"}
tbl = b"".join(pad4(node(k, u16(val), 1, vlen=len(val) + 1)) for k, val in strs.items())
st = node("040904B0", b"", 1, tbl, vlen=0)
sfi = node("StringFileInfo", b"", 1, pad4(st), vlen=0)
var = node("Translation", struct.pack("<HH", 0x409, 0x4B0), 0, vlen=4)
vfi = node("VarFileInfo", b"", 1, pad4(var), vlen=0)
version = node("VS_VERSION_INFO", fixed, 0, pad4(sfi) + pad4(vfi), vlen=len(fixed))

# --- resources: (type, id) -> data
res = {}
for i, p in enumerate(pngs): res[(3, i + 1)] = p
grp = struct.pack("<HHH", 0, 1, len(sizes))
for i, s in enumerate(sizes):
    grp += struct.pack("<BBBBHHIH", 0 if s >= 256 else s, 0 if s >= 256 else s, 0, 0, 1, 32, len(pngs[i]), i + 1)
res[(14, 1)] = grp
res[(24, 1)] = manifest
res[(16, 1)] = version

types = sorted({t for t, _ in res})
# layout: root dir, type dirs, name dirs, lang dirs, data entries, raw data
def dir_size(n): return 16 + 8 * n
offs = {}
pos = dir_size(len(types))
type_off = {}
for t in types:
    names = sorted(i for (tt, i) in res if tt == t)
    type_off[t] = pos
    pos += dir_size(len(names))
name_off = {}
for (t, i) in sorted(res):
    name_off[(t, i)] = pos
    pos += dir_size(1)
entry_off = {}
for k in sorted(res):
    entry_off[k] = pos
    pos += 16
data_off = {}
for k in sorted(res):
    data_off[k] = pos
    pos += (len(res[k]) + 3) // 4 * 4
total = pos

buf = bytearray(total)
relocs = []
def put_dir(off, entries):  # entries: list of (id, target, is_dir)
    struct.pack_into("<IIHHHH", buf, off, 0, 0, 0, 0, 0, len(entries))
    for j, (ident, target, isdir) in enumerate(entries):
        struct.pack_into("<II", buf, off + 16 + 8 * j, ident, target | (0x80000000 if isdir else 0))
put_dir(0, [(t, type_off[t], True) for t in types])
for t in types:
    names = sorted(i for (tt, i) in res if tt == t)
    put_dir(type_off[t], [(i, name_off[(t, i)], True) for i in names])
for k in sorted(res):
    put_dir(name_off[k], [(0x409, entry_off[k], False)])
    struct.pack_into("<IIII", buf, entry_off[k], data_off[k], len(res[k]), 0, 0)
    relocs.append(entry_off[k])
    buf[data_off[k]:data_off[k] + len(res[k])] = res[k]

nrel = len(relocs)
hdr_end = 20 + 40
raw_ptr = hdr_end
rel_ptr = raw_ptr + total
sym_ptr = rel_ptr + 10 * nrel
out = struct.pack("<HHIIIHH", 0x8664, 1, 0, sym_ptr, 2, 0, 0)
out += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, total, raw_ptr, rel_ptr, 0, nrel, 0, 0xC0000040)
out += bytes(buf)
for r in relocs:
    out += struct.pack("<IIH", r, 0, 3)  # IMAGE_REL_AMD64_ADDR32NB against symbol 0
out += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 1)
out += struct.pack("<IHHIHBBBB", total, nrel, 0, 0, 0, 0, 0, 0, 0)[:18]
out += struct.pack("<I", 4)
open("rsrc_windows_amd64.syso", "wb").write(out)
print("wrote rsrc_windows_amd64.syso", len(out), "bytes;", len(res), "resources")
