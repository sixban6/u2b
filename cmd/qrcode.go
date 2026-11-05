package cmd

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"strings"
)

// qrVersionSpec holds ISO/IEC 18004 Level-L parameters for QR versions 1..8 (up to 192 bytes).
type qrVersionSpec struct {
	version        int
	size           int
	dataCodewords  int
	ecPerBlock     int
	numBlocks      int
	alignCenters   []int
	versionInfoBit uint32 // 18-bit BCH for version >= 7
}

var qrSpecsL = []qrVersionSpec{
	{version: 1, size: 21, dataCodewords: 19, ecPerBlock: 7, numBlocks: 1, alignCenters: nil},
	{version: 2, size: 25, dataCodewords: 34, ecPerBlock: 10, numBlocks: 1, alignCenters: []int{6, 18}},
	{version: 3, size: 29, dataCodewords: 55, ecPerBlock: 15, numBlocks: 1, alignCenters: []int{6, 22}},
	{version: 4, size: 33, dataCodewords: 80, ecPerBlock: 20, numBlocks: 1, alignCenters: []int{6, 26}},
	{version: 5, size: 37, dataCodewords: 108, ecPerBlock: 26, numBlocks: 1, alignCenters: []int{6, 30}},
	{version: 6, size: 41, dataCodewords: 136, ecPerBlock: 18, numBlocks: 2, alignCenters: []int{6, 34}},
	{version: 7, size: 45, dataCodewords: 156, ecPerBlock: 20, numBlocks: 2, alignCenters: []int{6, 22, 38}, versionInfoBit: 0x07C94},
	{version: 8, size: 49, dataCodewords: 194, ecPerBlock: 24, numBlocks: 2, alignCenters: []int{6, 24, 42}, versionInfoBit: 0x085BC},
}

// Precomputed 15-bit format info for ECC Level L (01) and masks 0..7.
var qrFormatBitsL = [8]uint16{
	0x77C4, 0x72F3, 0x7DAA, 0x789D,
	0x662F, 0x6318, 0x6C41, 0x6976,
}

// EncodeQRMatrix encodes data into a boolean QR matrix (true = dark module).
func EncodeQRMatrix(text string) ([][]bool, error) {
	payload := []byte(text)
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty QR payload")
	}

	var spec *qrVersionSpec
	for i := range qrSpecsL {
		// Byte mode header: 4 bits mode (0100) + 8 bits length (for v1..9) = 12 bits -> max dataCodewords - 2 bytes
		maxBytes := qrSpecsL[i].dataCodewords - 2
		if len(payload) <= maxBytes {
			spec = &qrSpecsL[i]
			break
		}
	}
	if spec == nil {
		return nil, fmt.Errorf("QR payload too long (%d bytes, max 192)", len(payload))
	}

	dataCW := buildQRDataCodewords(payload, spec.dataCodewords)
	finalCW := interleaveQRBlocks(dataCW, spec.numBlocks, spec.ecPerBlock)

	n := spec.size
	modules := make([][]bool, n)
	isFunc := make([][]bool, n)
	for y := 0; y < n; y++ {
		modules[y] = make([]bool, n)
		isFunc[y] = make([]bool, n)
	}

	setFunc := func(x, y int, dark bool) {
		if x >= 0 && x < n && y >= 0 && y < n {
			modules[y][x] = dark
			isFunc[y][x] = true
		}
	}

	// Finder patterns + separators
	drawFinder := func(ox, oy int) {
		for dy := -1; dy <= 7; dy++ {
			for dx := -1; dx <= 7; dx++ {
				x, y := ox+dx, oy+dy
				if x < 0 || x >= n || y < 0 || y >= n {
					continue
				}
				dark := false
				if dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6 {
					if dx == 0 || dx == 6 || dy == 0 || dy == 6 || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4) {
						dark = true
					}
				}
				setFunc(x, y, dark)
			}
		}
	}
	drawFinder(0, 0)
	drawFinder(n-7, 0)
	drawFinder(0, n-7)

	// Timing patterns
	for i := 8; i < n-8; i++ {
		setFunc(i, 6, i%2 == 0)
		setFunc(6, i, i%2 == 0)
	}

	// Alignment patterns
	for _, cy := range spec.alignCenters {
		for _, cx := range spec.alignCenters {
			// Skip overlap with the 3 finder corners
			if (cx == 6 && cy == 6) || (cx == 6 && cy == n-7) || (cx == n-7 && cy == 6) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					dist := absInt(dx)
					if absInt(dy) > dist {
						dist = absInt(dy)
					}
					setFunc(cx+dx, cy+dy, dist != 1)
				}
			}
		}
	}

	// Dark module + reserve format areas
	setFunc(8, n-8, true)
	for i := 0; i <= 8; i++ {
		if !isFunc[8][i] {
			setFunc(i, 8, false)
		}
		if !isFunc[i][8] {
			setFunc(8, i, false)
		}
	}
	for i := 0; i < 8; i++ {
		if !isFunc[8][n-1-i] {
			setFunc(n-1-i, 8, false)
		}
		if i < 7 && !isFunc[n-1-i][8] {
			setFunc(8, n-1-i, false)
		}
	}

	// Version info (v7+)
	if spec.version >= 7 {
		for i := 0; i < 18; i++ {
			bit := ((spec.versionInfoBit >> i) & 1) != 0
			a := n - 11 + (i % 3)
			b := i / 3
			setFunc(b, a, bit)
			setFunc(a, b, bit)
		}
	}

	// Zig-zag codeword placement
	bitIdx := 0
	totalBits := len(finalCW) * 8
	upward := true
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < n; vert++ {
			y := vert
			if upward {
				y = n - 1 - vert
			}
			for j := 0; j < 2; j++ {
				x := right - j
				if isFunc[y][x] {
					continue
				}
				dark := false
				if bitIdx < totalBits {
					dark = ((finalCW[bitIdx>>3] >> (7 - (bitIdx & 7))) & 1) != 0
					bitIdx++
				}
				modules[y][x] = dark
			}
		}
		upward = !upward
	}

	// Choose lowest-penalty mask among 0..7
	bestMask := 0
	bestScore := int(^uint(0) >> 1)
	var bestGrid [][]bool

	for mask := 0; mask < 8; mask++ {
		cand := cloneBoolGrid(modules)
		applyQRMask(cand, isFunc, mask)
		writeQRFormatBits(cand, mask)
		score := qrPenaltyScore(cand)
		if score < bestScore {
			bestScore = score
			bestMask = mask
			bestGrid = cand
		}
	}
	_ = bestMask
	return bestGrid, nil
}

func buildQRDataCodewords(payload []byte, capacity int) []byte {
	var bits []bool
	appendBits := func(val int, count int) {
		for i := count - 1; i >= 0; i-- {
			bits = append(bits, ((val>>i)&1) != 0)
		}
	}

	// Mode indicator: Byte = 0100
	appendBits(0b0100, 4)
	appendBits(len(payload), 8)
	for _, b := range payload {
		appendBits(int(b), 8)
	}

	maxBits := capacity * 8
	for i := 0; i < 4 && len(bits) < maxBits; i++ {
		bits = append(bits, false)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}

	out := make([]byte, 0, capacity)
	for i := 0; i < len(bits); i += 8 {
		var b byte
		for j := 0; j < 8; j++ {
			b <<= 1
			if bits[i+j] {
				b |= 1
			}
		}
		out = append(out, b)
	}

	padBytes := [2]byte{0xEC, 0x11}
	for i := 0; len(out) < capacity; i++ {
		out = append(out, padBytes[i%2])
	}
	return out
}

func interleaveQRBlocks(data []byte, numBlocks, ecPerBlock int) []byte {
	blockSize := len(data) / numBlocks
	dataBlocks := make([][]byte, numBlocks)
	ecBlocks := make([][]byte, numBlocks)
	gen := rsGeneratorPoly(ecPerBlock)

	for b := 0; b < numBlocks; b++ {
		start := b * blockSize
		blk := data[start : start+blockSize]
		dataBlocks[b] = blk
		ecBlocks[b] = rsComputeRemainder(blk, gen)
	}

	var result []byte
	for i := 0; i < blockSize; i++ {
		for b := 0; b < numBlocks; b++ {
			result = append(result, dataBlocks[b][i])
		}
	}
	for i := 0; i < ecPerBlock; i++ {
		for b := 0; b < numBlocks; b++ {
			result = append(result, ecBlocks[b][i])
		}
	}
	return result
}

func gfMul(x, y byte) byte {
	var z byte
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x1D)
		z ^= ((y >> i) & 1) * x
	}
	return z
}

func rsGeneratorPoly(degree int) []byte {
	poly := make([]byte, degree)
	poly[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := 0; j < degree; j++ {
			poly[j] = gfMul(poly[j], root)
			if j+1 < degree {
				poly[j] ^= poly[j+1]
			}
		}
		root = gfMul(root, 0x02)
	}
	return poly
}

func rsComputeRemainder(data, gen []byte) []byte {
	rem := make([]byte, len(gen))
	for _, b := range data {
		factor := b ^ rem[0]
		copy(rem, rem[1:])
		rem[len(rem)-1] = 0
		for i, c := range gen {
			rem[i] ^= gfMul(c, factor)
		}
	}
	return rem
}

func applyQRMask(grid, isFunc [][]bool, mask int) {
	n := len(grid)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if isFunc[y][x] {
				continue
			}
			invert := false
			switch mask {
			case 0:
				invert = (x+y)%2 == 0
			case 1:
				invert = y%2 == 0
			case 2:
				invert = x%3 == 0
			case 3:
				invert = (x+y)%3 == 0
			case 4:
				invert = (y/2+x/3)%2 == 0
			case 5:
				invert = (x*y)%2+(x*y)%3 == 0
			case 6:
				invert = ((x*y)%2+(x*y)%3)%2 == 0
			case 7:
				invert = ((x+y)%2+(x*y)%3)%2 == 0
			}
			if invert {
				grid[y][x] = !grid[y][x]
			}
		}
	}
}

func writeQRFormatBits(grid [][]bool, mask int) {
	n := len(grid)
	bits := qrFormatBitsL[mask]

	for i := 0; i <= 5; i++ {
		grid[i][8] = ((bits >> i) & 1) != 0
	}
	grid[7][8] = ((bits >> 6) & 1) != 0
	grid[8][8] = ((bits >> 7) & 1) != 0
	grid[8][7] = ((bits >> 8) & 1) != 0
	for i := 9; i < 15; i++ {
		grid[8][14-i] = ((bits >> i) & 1) != 0
	}

	for i := 0; i < 8; i++ {
		grid[8][n-1-i] = ((bits >> i) & 1) != 0
	}
	for i := 8; i < 15; i++ {
		grid[n-15+i][8] = ((bits >> i) & 1) != 0
	}
	grid[n-8][8] = true
}

func qrPenaltyScore(grid [][]bool) int {
	n := len(grid)
	score := 0

	// Adjacent modules in row/col
	for y := 0; y < n; y++ {
		runColor := grid[y][0]
		runLen := 1
		for x := 1; x < n; x++ {
			if grid[y][x] == runColor {
				runLen++
			} else {
				if runLen >= 5 {
					score += 3 + (runLen - 5)
				}
				runColor = grid[y][x]
				runLen = 1
			}
		}
		if runLen >= 5 {
			score += 3 + (runLen - 5)
		}
	}
	for x := 0; x < n; x++ {
		runColor := grid[0][x]
		runLen := 1
		for y := 1; y < n; y++ {
			if grid[y][x] == runColor {
				runLen++
			} else {
				if runLen >= 5 {
					score += 3 + (runLen - 5)
				}
				runColor = grid[y][x]
				runLen = 1
			}
		}
		if runLen >= 5 {
			score += 3 + (runLen - 5)
		}
	}

	// 2x2 blocks
	for y := 0; y < n-1; y++ {
		for x := 0; x < n-1; x++ {
			c := grid[y][x]
			if grid[y][x+1] == c && grid[y+1][x] == c && grid[y+1][x+1] == c {
				score += 3
			}
		}
	}
	return score
}

func cloneBoolGrid(src [][]bool) [][]bool {
	out := make([][]bool, len(src))
	for i := range src {
		out[i] = make([]bool, len(src[i]))
		copy(out[i], src[i])
	}
	return out
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// RenderQRSVG returns a crisp, self-contained SVG string for the given QR payload.
func RenderQRSVG(text string) (string, error) {
	grid, err := EncodeQRMatrix(text)
	if err != nil {
		return "", err
	}
	n := len(grid)
	margin := 3
	total := n + margin*2

	var buf bytes.Buffer
	fmt.Fprintf(&buf, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, total, total)
	fmt.Fprintf(&buf, `<rect width="%d" height="%d" fill="#ffffff" rx="1.5"/>`, total, total)
	buf.WriteString(`<path fill="#0f172a" d="`)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if grid[y][x] {
				fmt.Fprintf(&buf, "M%d,%dh1v1h-1z", x+margin, y+margin)
			}
		}
	}
	buf.WriteString(`"/></svg>`)
	return buf.String(), nil
}

// RenderQRTerminal renders a compact Unicode half-block QR code for terminal display.
func RenderQRTerminal(text string) (string, error) {
	grid, err := EncodeQRMatrix(text)
	if err != nil {
		return "", err
	}
	n := len(grid)
	margin := 2
	total := n + margin*2

	isDark := func(x, y int) bool {
		gx, gy := x-margin, y-margin
		if gx < 0 || gx >= n || gy < 0 || gy >= n {
			return false
		}
		return grid[gy][gx]
	}

	var sb strings.Builder
	for y := 0; y < total; y += 2 {
		sb.WriteString("   ")
		for x := 0; x < total; x++ {
			top := isDark(x, y)
			bot := false
			if y+1 < total {
				bot = isDark(x, y+1)
			}
			// Invert visually (white background border around dark QR modules) so both dark & light terminals scan reliably
			switch {
			case !top && !bot:
				sb.WriteRune('█')
			case !top && bot:
				sb.WriteRune('▀')
			case top && !bot:
				sb.WriteRune('▄')
			default:
				sb.WriteRune(' ')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// RenderQRPNGImage creates an in-memory image.Image for testing / PNG export.
func RenderQRPNGImage(text string, scale int) (image.Image, error) {
	grid, err := EncodeQRMatrix(text)
	if err != nil {
		return nil, err
	}
	if scale < 1 {
		scale = 8
	}
	n := len(grid)
	margin := 4
	total := (n + margin*2) * scale
	img := image.NewNRGBA(image.Rect(0, 0, total, total))

	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	for y := 0; y < total; y++ {
		for x := 0; x < total; x++ {
			gx := x/scale - margin
			gy := y/scale - margin
			if gx >= 0 && gx < n && gy >= 0 && gy < n && grid[gy][gx] {
				img.SetNRGBA(x, y, black)
			} else {
				img.SetNRGBA(x, y, white)
			}
		}
	}
	return img, nil
}
