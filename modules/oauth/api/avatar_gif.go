package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"io"
	"math"
	"net/http"

	"golang.org/x/image/draw"
)

type avatarCrop struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Inspect GIF blocks before DecodeAll allocates any frame buffers.
func checkGIFBudget(data []byte) error {
	bad := fmt.Errorf("invalid or oversized GIF")
	if len(data) < 13 || len(data) > maxAvatarBytes || (string(data[:6]) != "GIF89a" && string(data[:6]) != "GIF87a") {
		return bad
	}
	width, height := int(binary.LittleEndian.Uint16(data[6:8])), int(binary.LittleEndian.Uint16(data[8:10]))
	pixels := int64(width) * int64(height)
	if width == 0 || height == 0 || width > 8192 || height > 8192 || pixels > 16_000_000 {
		return bad
	}
	pos := 13
	if data[10]&128 != 0 {
		pos += 3 * (1 << ((data[10] & 7) + 1))
	}
	skipBlocks := func() bool {
		for pos < len(data) {
			n := int(data[pos])
			pos++
			if n == 0 {
				return true
			}
			pos += n
		}
		return false
	}
	frames := 0
	for pos < len(data) {
		kind := data[pos]
		pos++
		switch kind {
		case 0x3b:
			if frames == 0 {
				return bad
			}
			return nil
		case 0x21:
			if pos >= len(data) {
				return bad
			}
			pos++
			if !skipBlocks() {
				return bad
			}
		case 0x2c:
			if pos+9 > len(data) {
				return bad
			}
			x, y := int(binary.LittleEndian.Uint16(data[pos:])), int(binary.LittleEndian.Uint16(data[pos+2:]))
			w, h := int(binary.LittleEndian.Uint16(data[pos+4:])), int(binary.LittleEndian.Uint16(data[pos+6:]))
			if w == 0 || h == 0 || x+w > width || y+h > height {
				return bad
			}
			packed := data[pos+8]
			pos += 9
			if packed&128 != 0 {
				pos += 3 * (1 << ((packed & 7) + 1))
			}
			if pos >= len(data) {
				return bad
			}
			pos++
			if !skipBlocks() {
				return bad
			}
			frames++
			if frames > 200 || pixels*int64(frames) > 40_000_000 {
				return bad
			}
		default:
			return bad
		}
	}
	return bad
}

// Re-rendering an animated avatar is CPU and memory heavy, so only a few run
// at once; further requests wait holding just their encoded bytes.
var gifWork = make(chan struct{}, 2)

func normalizeGIF(ctx context.Context, data []byte, crop *avatarCrop) ([]byte, error) {
	if err := checkGIFBudget(data); err != nil {
		return nil, err
	}
	// A request that timed out or was abandoned stops waiting for its turn.
	select {
	case gifWork <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-gifWork }()
	animation, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	w, h := animation.Config.Width, animation.Config.Height
	edge := min(w, h)
	rect := image.Rect((w-edge)/2, (h-edge)/2, (w-edge)/2+edge, (h-edge)/2+edge)
	if crop != nil {
		for _, v := range []float64{crop.X, crop.Y, crop.Width, crop.Height} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("invalid crop")
			}
		}
		if crop.X < 0 || crop.Y < 0 || crop.Width < 1 || crop.Height < 1 || crop.X+crop.Width > float64(w)+1 || crop.Y+crop.Height > float64(h)+1 || math.Abs(crop.Width-crop.Height) > 1 {
			return nil, fmt.Errorf("invalid crop")
		}
		rect = image.Rect(int(math.Round(crop.X)), int(math.Round(crop.Y)), min(w, int(math.Round(crop.X+crop.Width))), min(h, int(math.Round(crop.Y+crop.Height))))
		if rect.Empty() {
			return nil, fmt.Errorf("empty crop")
		}
	}
	size := min(512, rect.Dx(), rect.Dy())
	colors := color.Palette{color.Transparent}
	// Reuse exact source colors when possible; reserve index zero for transparency.
	seen := map[color.RGBA]bool{}
	for _, frame := range animation.Image {
		for _, c := range frame.Palette {
			r, g, b, a := c.RGBA()
			if a == 0 {
				continue
			}
			v := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
			if !seen[v] {
				seen[v] = true
				colors = append(colors, v)
			}
		}
	}
	if len(colors) > 256 {
		colors = append(color.Palette{color.Transparent}, palette.Plan9[:255]...)
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	result := &gif.GIF{LoopCount: animation.LoopCount, Config: image.Config{ColorModel: colors, Width: size, Height: size}, BackgroundIndex: 0}
	for i, frame := range animation.Image {
		disposal := byte(0)
		if i < len(animation.Disposal) {
			disposal = animation.Disposal[i]
		}
		var previous *image.NRGBA
		if disposal == gif.DisposalPrevious {
			previous = image.NewNRGBA(canvas.Bounds())
			copy(previous.Pix, canvas.Pix)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		scaled := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), canvas, rect, draw.Src, nil)
		output := image.NewPaletted(scaled.Bounds(), colors)
		draw.FloydSteinberg.Draw(output, output.Bounds(), scaled, image.Point{})
		result.Image = append(result.Image, output)
		result.Delay = append(result.Delay, animation.Delay[i])
		result.Disposal = append(result.Disposal, gif.DisposalBackground)
		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = previous
		}
	}
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, result); err != nil {
		return nil, err
	}
	if out.Len() > maxAvatarBytes {
		return nil, fmt.Errorf("cropped GIF exceeds 5 MB")
	}
	return out.Bytes(), nil
}

func (a *API) previewAvatarGIF(w http.ResponseWriter, r *http.Request) {
	original := r.Body
	defer func() { _, _ = io.Copy(io.Discard, io.LimitReader(original, maxAvatarBytes+(64<<10)+1)) }()
	if _, ok := a.AccountActor(w, r); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+(64<<10))
	fail := func() {
		writeJSON(w, 400, map[string]string{"error": "invalid_avatar", "error_description": "无法处理这张 GIF。请检查裁剪范围，图片不超过 5 MB、200 帧，全部画面合计不超过 4000 万像素。"})
	}
	if err := r.ParseMultipartForm(maxAvatarBytes + (64 << 10)); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		fail()
		return
	}
	defer r.MultipartForm.RemoveAll()
	var crop avatarCrop
	decoder := json.NewDecoder(bytes.NewBufferString(r.FormValue("crop")))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&crop) != nil || decoder.Decode(&struct{}{}) != io.EOF || len(r.MultipartForm.File["file"]) != 1 {
		fail()
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		fail()
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		fail()
		return
	}
	output, err := normalizeGIF(r.Context(), data, &crop)
	if r.Context().Err() != nil {
		// The reply was already taken over by the timeout, or nobody is waiting.
		return
	}
	if err != nil {
		fail()
		return
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(output)
}
