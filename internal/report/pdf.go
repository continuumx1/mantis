package report

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Contents selects report sections.
type Contents struct {
	Topology  bool `json:"topology"`
	Inventory bool `json:"inventory"`
	Health    bool `json:"health"`
	Findings  bool `json:"findings"`
}

type rgb [3]int

var (
	cNavy      = rgb{17, 24, 39}
	cTeal      = rgb{13, 148, 136}
	cText      = rgb{23, 32, 51}
	cMuted     = rgb{100, 116, 139}
	cFaint     = rgb{148, 163, 184}
	cBorder    = rgb{226, 232, 240}
	cSurf      = rgb{248, 250, 252}
	cWhite     = rgb{255, 255, 255}
	levelColor = map[Level]rgb{Healthy: {22, 163, 74}, Warning: {217, 119, 6}, Critical: {220, 38, 38}, Unknown: {100, 116, 139}, Informational: {37, 99, 235}}
	// Resource and relationship colors: the app's light-theme palette.
	catColor = map[string]rgb{
		"workload": {47, 111, 224}, "pod": {79, 138, 28}, "network": {162, 63, 209}, "config": {160, 100, 0},
		"storage": {10, 143, 124}, "node": {91, 102, 112}, "autoscale": {184, 92, 23}, "security": {176, 48, 111}, "other": {100, 116, 139},
	}
	relColor = map[string]rgb{
		"ownership": catColor["workload"], "networking": catColor["network"], "configuration": catColor["config"], "storage": catColor["storage"],
		"scheduling": catColor["node"], "security": catColor["security"], "autoscaling": catColor["autoscale"],
	}
	relDash = map[string][]float64{"configuration": {5, 4}, "storage": {9, 3, 2, 3}, "scheduling": {1.5, 4}, "security": {2, 3}, "autoscaling": {3, 3}}
	kindCat = map[string]string{
		"Deployment": "workload", "ReplicaSet": "workload", "StatefulSet": "workload", "DaemonSet": "workload", "CronJob": "workload", "Job": "workload",
		"Pod": "pod", "Service": "network", "Ingress": "network", "NetworkPolicy": "network",
		"ConfigMap": "config", "Secret": "config", "ResourceQuota": "config", "LimitRange": "config",
		"PersistentVolumeClaim": "storage", "PersistentVolume": "storage", "StorageClass": "storage", "Node": "node",
		"HorizontalPodAutoscaler": "autoscale", "VerticalPodAutoscaler": "autoscale", "NodePool": "autoscale",
		"ServiceAccount": "security", "Role": "security", "RoleBinding": "security", "ClusterRole": "security", "ClusterRoleBinding": "security",
	}
)

const (
	pageW, pageH = 842.0, 595.0 // A4 landscape, points
	margin       = 36.0
	bodyTop      = 62.0
	bodyBottom   = pageH - 40
	// PDF viewers handle pages up to 200 in (14,400 pt) per side.
	maxPageDim = 14400.0
	// 1 layout pixel is drawn as 0.75 pt (the CSS px→pt ratio): 12 px card
	// text becomes 9 pt, readable when printed and sharp when zoomed.
	baseScale = 0.75
	// Below this a diagram is no longer readable on its own page; the UI
	// partitions by namespace instead of submitting such a page.
	minScale = 0.3
)

type doc struct {
	pdf     *fpdf.Fpdf
	tr      func(string) string
	snap    *Snapshot
	title   string
	links   map[string]int
	aliases map[string]string
	section string
	// pagesIndex[i] is the namespace topology page i shows ("" if none), for
	// continuation references between partitioned diagrams.
	pagesIndex []string
}

type tocEntry struct {
	key, title string
	level      int
}

// Build renders the report PDF (unencrypted; see Encrypt). progress is called
// at each stage.
func Build(s *Snapshot, pages []*page, c Contents, progress func(string)) ([]byte, error) {
	if progress == nil {
		progress = func(string) {}
	}
	pdf := fpdf.NewCustom(&fpdf.InitType{UnitStr: "pt", Size: fpdf.SizeType{Wd: pageW, Ht: pageH}, OrientationStr: "P"})
	pdf.SetCompression(true)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetMargins(margin, bodyTop, margin)
	pdf.AliasNbPages("{nb}")
	d := &doc{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor(""), snap: s, links: map[string]int{}, aliases: map[string]string{}}
	cluster := s.Graph.Context
	if cluster == "" {
		cluster = "cluster"
	}
	d.title = "Mantis Infrastructure Report · " + cluster
	for _, p := range pages {
		d.pagesIndex = append(d.pagesIndex, p.Namespace)
	}
	pdf.SetTitle(d.tr("Mantis Infrastructure Report — "+cluster), false)
	pdf.SetAuthor("Mantis", false)
	pdf.SetCreator("Mantis (mantis-report)", false)
	pdf.SetSubject("Kubernetes cluster topology, inventory, health and findings", false)
	pdf.SetHeaderFuncMode(d.header, false)
	pdf.SetFooterFunc(d.footer)

	h := ComputeHealth(s)
	findings := ComputeFindings(s, h)

	var toc []tocEntry
	toc = append(toc, tocEntry{"overview", "Cluster Overview", 0})
	if c.Health {
		toc = append(toc, tocEntry{"health", "Cluster Health Summary", 0})
	}
	if c.Topology && len(pages) > 0 {
		toc = append(toc, tocEntry{"topology", "Complete Cluster Topology", 0})
		for i, p := range pages {
			toc = append(toc, tocEntry{"topo:" + strconv.Itoa(i), p.Title, 1})
		}
	}
	if c.Inventory {
		toc = append(toc,
			tocEntry{"inv-ns", "Namespace-wise Resource Inventory", 0},
			tocEntry{"inv-wl", "Workload Inventory", 0},
			tocEntry{"inv-pods", "Pod Inventory", 0},
			tocEntry{"inv-net", "Networking and Service Inventory", 0},
			tocEntry{"inv-store", "Storage and Configuration Inventory", 0},
			tocEntry{"inv-other", "Other Supported Resources", 0})
	}
	if c.Findings {
		toc = append(toc, tocEntry{"findings", "Findings and Anomalies", 0})
	}
	toc = append(toc, tocEntry{"meta", "Report Metadata", 0})
	for _, e := range toc {
		d.links[e.key] = pdf.AddLink()
	}

	d.cover(h, len(findings))
	d.tocPages(toc)

	d.start("overview", "Cluster Overview", 0)
	d.overview(h)
	if c.Health {
		d.start("health", "Cluster Health Summary", 0)
		d.health(h)
	}
	if c.Topology && len(pages) > 0 {
		progress("rendering-topology")
		for i, p := range pages {
			d.topologyPage(p, i, len(pages))
		}
	}
	if c.Inventory {
		progress("generating-inventory")
		d.inventory()
	}
	if c.Findings {
		d.start("findings", "Findings and Anomalies", 0)
		d.findings(findings)
	}
	d.start("meta", "Report Metadata", 0)
	d.metadata(c, len(pages), len(findings))

	progress("rendering-pdf")
	for a, r := range d.aliases {
		pdf.RegisterAlias(a, r)
	}
	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------- chrome

func (d *doc) color(fill bool, c rgb) {
	if fill {
		d.pdf.SetFillColor(c[0], c[1], c[2])
	} else {
		d.pdf.SetDrawColor(c[0], c[1], c[2])
	}
}
func (d *doc) text(c rgb) { d.pdf.SetTextColor(c[0], c[1], c[2]) }

func (d *doc) header() {
	if d.pdf.PageNo() == 1 {
		return
	}
	w, _ := d.pdf.GetPageSize()
	d.color(true, cTeal)
	d.pdf.Rect(0, 0, w, 3, "F")
	d.pdf.SetFont("Helvetica", "B", 8.5)
	d.text(cText)
	d.pdf.Text(margin, 24, d.tr("Mantis Infrastructure Report"))
	d.pdf.SetFont("Helvetica", "", 8.5)
	d.text(cMuted)
	right := d.tr(d.snap.Graph.Context + " · " + d.section)
	d.pdf.Text(w-margin-d.pdf.GetStringWidth(right), 24, right)
	d.color(false, cBorder)
	d.pdf.SetLineWidth(0.6)
	d.pdf.Line(margin, 32, w-margin, 32)
}

func (d *doc) footer() {
	if d.pdf.PageNo() == 1 {
		return
	}
	w, h := d.pdf.GetPageSize()
	d.pdf.SetFont("Helvetica", "", 7.5)
	d.text(cFaint)
	left := d.tr(fmt.Sprintf("Generated %s · graph revision %d · Confidential: describes cluster infrastructure",
		d.snap.TakenAt.Format("2006-01-02 15:04 MST"), d.snap.Graph.Revision))
	d.pdf.Text(margin, h-18, left)
	pg := fmt.Sprintf("Page %d of {nb}", d.pdf.PageNo())
	d.pdf.Text(w-margin-d.pdf.GetStringWidth(fmt.Sprintf("Page %d of 999", d.pdf.PageNo())), h-18, pg)
}

// start begins a section on a new A4 page, registers its TOC link, bookmark
// and page-number alias.
func (d *doc) start(key, title string, level int) {
	d.section = title
	d.pdf.AddPageFormat("P", fpdf.SizeType{Wd: pageW, Ht: pageH})
	d.mark(key, title, level)
	d.pdf.SetFont("Helvetica", "B", 18)
	d.text(cText)
	d.pdf.Text(margin, bodyTop+6, d.tr(title))
	d.pdf.SetY(bodyTop + 22)
}

func (d *doc) mark(key, title string, level int) {
	if l, ok := d.links[key]; ok {
		d.pdf.SetLink(l, 0, -1)
	}
	d.aliases["{p:"+key+"}"] = strconv.Itoa(d.pdf.PageNo())
	d.pdf.Bookmark(d.tr(title), level, 0)
}

func (d *doc) heading(t string) {
	d.need(40)
	y := d.pdf.GetY() + 8
	d.pdf.SetFont("Helvetica", "B", 12.5)
	d.text(cText)
	d.pdf.Text(margin, y+10, d.tr(t))
	d.pdf.SetY(y + 18)
}

func (d *doc) para(t string) {
	d.pdf.SetFont("Helvetica", "", 9)
	d.text(cMuted)
	d.pdf.SetX(margin)
	d.pdf.MultiCell(pageW-2*margin, 12, d.tr(t), "", "L", false)
	d.pdf.SetY(d.pdf.GetY() + 4)
}

// need starts a continuation page if fewer than h points remain.
func (d *doc) need(h float64) bool {
	if d.pdf.GetY()+h <= bodyBottom {
		return false
	}
	d.pdf.AddPageFormat("P", fpdf.SizeType{Wd: pageW, Ht: pageH})
	d.pdf.SetY(bodyTop)
	return true
}

// ---------------------------------------------------------------- cover & toc

func (d *doc) cover(h Health, findings int) {
	pdf := d.pdf
	pdf.AddPageFormat("P", fpdf.SizeType{Wd: pageW, Ht: pageH})
	d.color(true, cNavy)
	pdf.Rect(0, 0, pageW, pageH, "F")
	d.mantisMark(margin, 60, 46)
	pdf.SetFont("Helvetica", "B", 15)
	d.text(cWhite)
	pdf.Text(margin+58, 92, "Mantis")
	pdf.SetFont("Helvetica", "B", 34)
	pdf.Text(margin, 210, d.tr("Infrastructure Report"))
	pdf.SetFont("Helvetica", "", 15)
	d.text(rgb{203, 213, 225})
	pdf.Text(margin, 240, d.tr("Kubernetes cluster topology, inventory, health and findings"))
	rows := [][2]string{
		{"Cluster", d.snap.Graph.Context},
		{"Kubernetes", nz(d.snap.Graph.Version)},
		{"Generated", d.snap.TakenAt.Format("2 January 2006, 15:04 MST")},
		{"Graph revision", strconv.FormatUint(d.snap.Graph.Revision, 10)},
		{"Resources", fmt.Sprintf("%d in %d namespace%s on %d node%s", len(d.snap.Graph.Nodes), len(d.snap.Graph.Namespaces), plS(len(d.snap.Graph.Namespaces)), len(d.snap.Nodes), plS(len(d.snap.Nodes)))},
		{"Findings", strconv.Itoa(findings)},
	}
	y := 300.0
	for _, r := range rows {
		pdf.SetFont("Helvetica", "", 10)
		d.text(rgb{139, 152, 172})
		pdf.Text(margin, y, d.tr(r[0]))
		pdf.SetFont("Helvetica", "B", 11)
		d.text(cWhite)
		pdf.Text(margin+120, y, d.tr(r[1]))
		y += 22
	}
	pdf.SetFont("Helvetica", "", 8.5)
	d.text(rgb{139, 152, 172})
	pdf.Text(margin, pageH-40, d.tr("Confidential. This document describes cluster infrastructure and is protected with a password (AES-256)."))
	pdf.Text(margin, pageH-28, d.tr("Generated by Mantis from the cluster as observed at the time above. Secret and ConfigMap values are never included."))
}

// mantisMark draws the Mantis symbol (teal head, eyes, antennae) as vectors.
func (d *doc) mantisMark(x, y, size float64) {
	s := size / 32
	pt := func(px, py float64) fpdf.PointType { return fpdf.PointType{X: x + px*s, Y: y + py*s} }
	pdf := d.pdf
	d.color(true, cTeal)
	d.color(false, cTeal)
	pdf.SetLineWidth(1.8 * s)
	pdf.SetLineCapStyle("round")
	pdf.CurveBezierCubic(x+12.6*s, y+8.2*s, x+11.4*s, y+5.4*s, x+9.6*s, y+3.6*s, x+7.2*s, y+2.6*s, "D")
	pdf.CurveBezierCubic(x+19.4*s, y+8.2*s, x+20.6*s, y+5.4*s, x+22.4*s, y+3.6*s, x+24.8*s, y+2.6*s, "D")
	pdf.Polygon([]fpdf.PointType{pt(5.2, 9.6), pt(26.8, 9.6), pt(16, 28.6)}, "F")
	d.color(true, rgb{204, 251, 241})
	pdf.Circle(x+10.2*s, y+11.4*s, 4.1*s, "F")
	pdf.Circle(x+21.8*s, y+11.4*s, 4.1*s, "F")
	d.color(true, cNavy)
	pdf.Circle(x+11*s, y+11.8*s, 1.7*s, "F")
	pdf.Circle(x+21*s, y+11.8*s, 1.7*s, "F")
	pdf.SetLineCapStyle("butt")
}

func (d *doc) tocPages(toc []tocEntry) {
	pdf := d.pdf
	d.section = "Contents"
	perPage := 26
	for i, e := range toc {
		if i%perPage == 0 {
			pdf.AddPageFormat("P", fpdf.SizeType{Wd: pageW, Ht: pageH})
			if i == 0 {
				pdf.Bookmark("Contents", 0, 0)
			}
			pdf.SetFont("Helvetica", "B", 18)
			d.text(cText)
			pdf.Text(margin, bodyTop+6, "Contents")
			pdf.SetY(bodyTop + 24)
		}
		indent := 0.0
		pdf.SetFont("Helvetica", "B", 10.5)
		d.text(cText)
		if e.level > 0 {
			indent = 18
			pdf.SetFont("Helvetica", "", 9.5)
			d.text(cMuted)
		}
		y := pdf.GetY()
		pdf.SetXY(margin+indent, y)
		pdf.CellFormat(pageW-2*margin-indent-60, 17, d.tr(e.title), "", 0, "L", false, d.links[e.key], "")
		pdf.CellFormat(60, 17, "{p:"+e.key+"}", "", 0, "R", false, d.links[e.key], "")
		d.color(false, cBorder)
		pdf.SetLineWidth(0.4)
		pdf.Line(margin+indent, y+17, pageW-margin, y+17)
		pdf.SetY(y + 18)
	}
}

// ---------------------------------------------------------------- tables

type col struct {
	title string
	w     float64 // fraction of usable width
	align string
	wrap  bool
	mono  bool
}

type cell struct {
	text  string
	level *Level
}

func txt(s string) cell          { return cell{text: s} }
func lvl(s string, l Level) cell { return cell{text: s, level: &l} }

// table draws rows with the header repeated on every page and long cells
// wrapped (wrap columns) or truncated with an ellipsis.
func (d *doc) table(cols []col, rows [][]cell, empty string) {
	pdf := d.pdf
	usable := pageW - 2*margin
	widths := make([]float64, len(cols))
	for i, c := range cols {
		widths[i] = c.w * usable
	}
	drawHeader := func() {
		y := pdf.GetY()
		d.color(true, cSurf)
		pdf.Rect(margin, y, usable, 18, "F")
		pdf.SetFont("Helvetica", "B", 8)
		d.text(cMuted)
		x := margin
		for i, c := range cols {
			pdf.SetXY(x+4, y)
			pdf.CellFormat(widths[i]-8, 18, d.tr(strings.ToUpper(c.title)), "", 0, alignOf(c.align), false, 0, "")
			x += widths[i]
		}
		d.color(false, cBorder)
		pdf.SetLineWidth(0.6)
		pdf.Line(margin, y+18, margin+usable, y+18)
		pdf.SetY(y + 18)
	}
	if len(rows) == 0 {
		d.need(24)
		pdf.SetFont("Helvetica", "I", 9)
		d.text(cMuted)
		pdf.SetX(margin)
		pdf.CellFormat(usable, 18, d.tr(empty), "", 1, "L", false, 0, "")
		pdf.SetY(pdf.GetY() + 6)
		return
	}
	d.need(40)
	drawHeader()
	const lineH = 11.0
	for ri, r := range rows {
		// Row height from wrapped columns.
		lines := make([][]string, len(cols))
		maxLines := 1
		for i, c := range cols {
			if i >= len(r) {
				continue
			}
			setCellFont(pdf, c, r[i])
			if c.wrap {
				ls := d.wrap(r[i].text, widths[i]-8)
				if len(ls) > 6 {
					ls = append(ls[:5], "…")
				}
				lines[i] = ls
				if len(ls) > maxLines {
					maxLines = len(ls)
				}
			}
		}
		rowH := float64(maxLines)*lineH + 7
		if pdf.GetY()+rowH > bodyBottom {
			d.need(bodyBottom)
			drawHeader()
		}
		y := pdf.GetY()
		if ri%2 == 1 {
			d.color(true, rgb{251, 252, 253})
			pdf.Rect(margin, y, usable, rowH, "F")
		}
		x := margin
		for i, c := range cols {
			if i >= len(r) {
				break
			}
			cl := r[i]
			setCellFont(pdf, c, cl)
			if cl.level != nil {
				col := levelColor[*cl.level]
				d.color(true, col)
				pdf.Circle(x+8, y+3.5+lineH/2, 2.6, "F")
				d.text(col)
				pdf.SetXY(x+14, y+3.5)
				pdf.CellFormat(widths[i]-18, lineH, d.fit(cl.text, widths[i]-18), "", 0, "L", false, 0, "")
			} else if c.wrap {
				d.text(cText)
				for li, l := range lines[i] {
					pdf.SetXY(x+4, y+3.5+float64(li)*lineH)
					pdf.CellFormat(widths[i]-8, lineH, l, "", 0, alignOf(c.align), false, 0, "")
				}
			} else {
				d.text(cText)
				pdf.SetXY(x+4, y+3.5)
				pdf.CellFormat(widths[i]-8, lineH, d.fit(cl.text, widths[i]-8), "", 0, alignOf(c.align), false, 0, "")
			}
			x += widths[i]
		}
		d.color(false, cBorder)
		pdf.SetLineWidth(0.4)
		pdf.Line(margin, y+rowH, margin+usable, y+rowH)
		pdf.SetY(y + rowH)
	}
	pdf.SetY(pdf.GetY() + 10)
}

func setCellFont(pdf *fpdf.Fpdf, c col, cl cell) {
	switch {
	case c.mono:
		pdf.SetFont("Courier", "", 8.2)
	case cl.level != nil:
		pdf.SetFont("Helvetica", "B", 8.5)
	default:
		pdf.SetFont("Helvetica", "", 8.5)
	}
}

func alignOf(a string) string {
	if a == "" {
		return "L"
	}
	return a
}

// wrap breaks s into lines no wider than w in the current font, returning
// them already translated for the PDF's core fonts. (fpdf's own SplitText
// indexes width tables by rune and cannot handle translated text.)
func (d *doc) wrap(s string, w float64) []string {
	var lines []string
	cur := ""
	width := func(t string) float64 { return d.pdf.GetStringWidth(d.tr(t)) }
	for _, word := range strings.Fields(s) {
		cand := word
		if cur != "" {
			cand = cur + " " + word
		}
		if width(cand) <= w {
			cur = cand
			continue
		}
		if cur != "" {
			lines = append(lines, d.tr(cur))
			cur = ""
		}
		// A single token wider than the column is broken by characters.
		for width(word) > w {
			r := []rune(word)
			n := len(r)
			for n > 1 && width(string(r[:n])) > w {
				n--
			}
			lines = append(lines, d.tr(string(r[:n])))
			word = string(r[n:])
		}
		cur = word
	}
	if cur != "" || len(lines) == 0 {
		lines = append(lines, d.tr(cur))
	}
	return lines
}

// fit truncates s with an ellipsis to fit width w in the current font.
func (d *doc) fit(s string, w float64) string {
	t := d.tr(s)
	if d.pdf.GetStringWidth(t) <= w {
		return t
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		if c := d.tr(string(r) + "..."); d.pdf.GetStringWidth(c) <= w {
			return c
		}
	}
	return ""
}

// kv draws a two-column key/value block.
func (d *doc) kv(rows [][2]string) {
	pdf := d.pdf
	for _, r := range rows {
		d.need(16)
		y := pdf.GetY()
		pdf.SetFont("Helvetica", "", 9)
		d.text(cMuted)
		pdf.SetXY(margin, y)
		pdf.CellFormat(170, 15, d.tr(r[0]), "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "B", 9)
		d.text(cText)
		pdf.SetXY(margin+170, y)
		pdf.MultiCell(pageW-2*margin-170, 15, d.tr(r[1]), "", "L", false)
		d.color(false, cBorder)
		pdf.SetLineWidth(0.4)
		pdf.Line(margin, pdf.GetY(), pageW-margin, pdf.GetY())
	}
	pdf.SetY(pdf.GetY() + 10)
}

// tiles draws a row of summary tiles.
func (d *doc) tiles(items []tile) {
	pdf := d.pdf
	d.need(70)
	y := pdf.GetY()
	gap := 10.0
	w := (pageW - 2*margin - gap*float64(len(items)-1)) / float64(len(items))
	for i, t := range items {
		x := margin + float64(i)*(w+gap)
		d.color(true, cWhite)
		d.color(false, cBorder)
		pdf.SetLineWidth(0.8)
		pdf.RoundedRect(x, y, w, 58, 5, "1234", "FD")
		c := levelColor[t.level]
		d.color(true, c)
		pdf.Rect(x, y+6, 3, 46, "F")
		pdf.SetFont("Helvetica", "", 8.5)
		d.text(cMuted)
		pdf.Text(x+12, y+18, d.tr(t.label))
		pdf.SetFont("Helvetica", "B", 19)
		d.text(c)
		pdf.Text(x+12, y+42, d.tr(t.value))
		pdf.SetFont("Helvetica", "", 7.8)
		d.text(cMuted)
		pdf.Text(x+12, y+53, d.fit(t.note, w-16))
	}
	pdf.SetY(y + 70)
}

type tile struct {
	label, value, note string
	level              Level
}

// bar draws a stacked distribution bar with a legend.
func (d *doc) bar(parts []barPart) {
	pdf := d.pdf
	d.need(40)
	y := pdf.GetY()
	total := 0
	for _, p := range parts {
		total += p.n
	}
	w := pageW - 2*margin
	x := margin
	d.color(true, cSurf)
	pdf.Rect(margin, y, w, 10, "F")
	if total > 0 {
		for _, p := range parts {
			if p.n == 0 {
				continue
			}
			pw := w * float64(p.n) / float64(total)
			d.color(true, p.c)
			pdf.Rect(x, y, pw, 10, "F")
			x += pw
		}
	}
	x = margin
	pdf.SetFont("Helvetica", "", 8.5)
	for _, p := range parts {
		d.color(true, p.c)
		pdf.Rect(x, y+17, 7, 7, "F")
		d.text(cText)
		label := d.tr(fmt.Sprintf("%s %d", p.label, p.n))
		pdf.Text(x+11, y+23.5, label)
		x += 22 + pdf.GetStringWidth(label)
	}
	pdf.SetY(y + 34)
}

type barPart struct {
	label string
	n     int
	c     rgb
}

// ---------------------------------------------------------------- sections

func (d *doc) overview(h Health) {
	s := d.snap
	kinds := map[string]int{}
	for _, n := range s.Graph.Nodes {
		if n.Resolved {
			kinds[n.Kind]++
		}
	}
	d.kv([][2]string{
		{"Cluster", s.Graph.Context},
		{"Cluster identifier", nz(s.Graph.ClusterID)},
		{"API server", nz(s.Graph.Server)},
		{"Kubernetes version", nz(s.Graph.Version)},
		{"Report generated", s.TakenAt.Format(time.RFC1123)},
		{"Nodes", strconv.Itoa(len(s.Nodes))},
		{"Namespaces", strconv.Itoa(len(s.Graph.Namespaces))},
		{"Resources in the graph", fmt.Sprintf("%d (%d relationships)", len(s.Graph.Nodes), len(s.Graph.Edges))},
	})
	d.heading("Workload distribution")
	d.tiles([]tile{
		{"Deployments", strconv.Itoa(len(s.Deployments)), fmt.Sprintf("%d ready", h.DeploymentsReady), levelOf(h.DeploymentsReady, h.DeploymentsTotal)},
		{"StatefulSets", strconv.Itoa(len(s.StatefulSets)), fmt.Sprintf("%d ready", h.StatefulSetsReady), levelOf(h.StatefulSetsReady, h.StatefulSetsTotal)},
		{"DaemonSets", strconv.Itoa(len(s.DaemonSets)), fmt.Sprintf("%d ready", h.DaemonSetsReady), levelOf(h.DaemonSetsReady, h.DaemonSetsTotal)},
		{"Jobs / CronJobs", fmt.Sprintf("%d / %d", len(s.Jobs), len(s.CronJobs)), "batch workloads", Unknown},
		{"Pods", strconv.Itoa(len(s.Pods)), fmt.Sprintf("%d running and ready", h.Pods["Running"]), levelOf(h.Pods["Running"]+h.Pods["Succeeded"], len(s.Pods))},
	})
	d.heading("Pod status distribution")
	d.bar(podParts(h))
	d.heading("Deployment readiness")
	d.para(fmt.Sprintf("%d of %d Deployments have all desired replicas ready and available.", h.DeploymentsReady, h.DeploymentsTotal))
	d.heading("Resource types included")
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	var rows [][]cell
	for _, k := range names {
		rows = append(rows, []cell{txt(k), txt(strconv.Itoa(kinds[k]))})
	}
	d.table([]col{{title: "Kind", w: .5}, {title: "Count", w: .5, align: "R"}}, rows, "No resources.")
}

func podParts(h Health) []barPart {
	return []barPart{
		{"Running", h.Pods["Running"], levelColor[Healthy]},
		{"Running, not ready", h.Pods["Running, not ready"], levelColor[Warning]},
		{"Pending", h.Pods["Pending"], rgb{245, 158, 11}},
		{"Failed", h.Pods["Failed"], levelColor[Critical]},
		{"Succeeded", h.Pods["Succeeded"], rgb{148, 163, 184}},
		{"Unknown", h.Pods["Unknown"], rgb{100, 116, 139}},
	}
}

func levelOf(ok, total int) Level {
	switch {
	case total == 0:
		return Unknown
	case ok == total:
		return Healthy
	case ok == 0:
		return Critical
	}
	return Warning
}

func (d *doc) health(h Health) {
	s := d.snap
	readyNodes := h.NodeLevels[Healthy]
	notReady := h.NodeLevels[Critical]
	d.para("Every indicator below is derived from observed Kubernetes status fields (phase, container readiness, replica counts, node conditions) at report time. A resource is never counted as healthy merely because it exists.")
	d.tiles([]tile{
		{"Running pods", strconv.Itoa(h.Pods["Running"]), fmt.Sprintf("of %d pods", len(s.Pods)), levelOf(h.Pods["Running"]+h.Pods["Succeeded"], len(s.Pods))},
		{"Pending", strconv.Itoa(h.Pods["Pending"]), "waiting to be scheduled or started", lvlIf(h.Pods["Pending"] > 0, Warning)},
		{"Failed", strconv.Itoa(h.Pods["Failed"] + h.Pods["Running, not ready"]), "failed, or running but not ready", lvlIf(h.Pods["Failed"]+h.Pods["Running, not ready"] > 0, Critical)},
		{"Unknown", strconv.Itoa(h.Pods["Unknown"]), "status not reported", lvlIf(h.Pods["Unknown"] > 0, Unknown)},
		{"With restarts", strconv.Itoa(h.PodsWithRestarts), "at least one container restart", lvlIf(h.PodsWithRestarts > 0, Warning)},
	})
	d.tiles([]tile{
		{"Workloads ready", fmt.Sprintf("%d / %d", h.WorkloadLevels[Healthy], len(h.Workloads)), "Deployments, StatefulSets, DaemonSets", levelOf(h.WorkloadLevels[Healthy], len(h.Workloads)-h.WorkloadLevels[Unknown])},
		{"Unavailable workloads", strconv.Itoa(h.WorkloadLevels[Critical] + h.WorkloadLevels[Warning]), "below desired replicas", lvlIf(h.WorkloadLevels[Critical] > 0, Critical, h.WorkloadLevels[Warning] > 0, Warning)},
		{"Nodes ready", fmt.Sprintf("%d / %d", readyNodes, len(h.Nodes)), "Ready condition True", levelOf(readyNodes, len(h.Nodes))},
		{"Nodes not ready", strconv.Itoa(notReady), "Ready condition False", lvlIf(notReady > 0, Critical)},
		{"Nodes unknown / pressure", fmt.Sprintf("%d / %d", h.NodeLevels[Unknown], h.NodeLevels[Warning]), "no status / pressure conditions", lvlIf(h.NodeLevels[Warning] > 0, Warning, h.NodeLevels[Unknown] > 0, Unknown)},
	})
	d.heading("Workload readiness")
	ws := append([]WorkloadState(nil), h.Workloads...)
	sort.SliceStable(ws, func(i, j int) bool { return sevRank(ws[i].Level) < sevRank(ws[j].Level) })
	var rows [][]cell
	for _, w := range ws {
		status := w.Level.String()
		if w.Note != "" {
			status += " — " + w.Note
		}
		rows = append(rows, []cell{txt(w.Kind), txt(w.Namespace), txt(w.Name), txt(i32(w.Desired)), txt(i32(w.Ready)), txt(i32(w.Available)), lvl(status, w.Level)})
	}
	d.table([]col{{title: "Kind", w: .1}, {title: "Namespace", w: .14}, {title: "Name", w: .26, mono: true}, {title: "Desired", w: .08, align: "R"}, {title: "Ready", w: .08, align: "R"}, {title: "Available", w: .09, align: "R"}, {title: "Status", w: .25}}, rows, "No Deployments, StatefulSets or DaemonSets.")
	d.heading("Node readiness")
	rows = nil
	for _, n := range h.Nodes {
		rows = append(rows, []cell{txt(n.Name), lvl(n.Ready, n.Level), txt(nz(n.Roles)), txt(nz(n.Version)), txt(nz(n.Pressure))})
	}
	d.table([]col{{title: "Node", w: .32, mono: true}, {title: "Ready", w: .14}, {title: "Roles", w: .16}, {title: "Kubelet", w: .14}, {title: "Pressure", w: .24}}, rows, "Node data is not available.")
	d.heading("Pods that are not running and ready")
	rows = nil
	for i := range s.Pods {
		st := ObservePod(&s.Pods[i])
		if st.Level == Healthy {
			continue
		}
		rows = append(rows, []cell{txt(s.Pods[i].Namespace), txt(s.Pods[i].Name), lvl(st.Phase+reasonSuffix(st.Reason), st.Level), txt(fmt.Sprintf("%d/%d", st.Ready, st.Containers)), txt(strconv.Itoa(int(st.Restarts))), txt(nz(st.Node))})
	}
	d.table([]col{{title: "Namespace", w: .14}, {title: "Pod", w: .3, mono: true}, {title: "Status", w: .22}, {title: "Ready", w: .08, align: "R"}, {title: "Restarts", w: .08, align: "R"}, {title: "Node", w: .18, mono: true}}, rows, "Every pod is running and ready (or completed).")
}

func lvlIf(args ...any) Level {
	for i := 0; i+1 < len(args); i += 2 {
		if args[i].(bool) {
			return args[i+1].(Level)
		}
	}
	return Healthy
}

func (d *doc) findings(fs []Finding) {
	counts := map[Level]int{}
	for _, f := range fs {
		counts[f.Severity]++
	}
	d.para("Findings are anomalies Mantis detects from the cluster's own state and the relationships it resolves: broken or unverifiable references, unavailable workloads, failing pods, Services without backends, unready nodes and unbound claims. Mantis does not perform security, performance or compliance assessments.")
	d.tiles([]tile{
		{"Critical", strconv.Itoa(counts[Critical]), "needs action", lvlIf(counts[Critical] > 0, Critical)},
		{"Warning", strconv.Itoa(counts[Warning]), "degraded or at risk", lvlIf(counts[Warning] > 0, Warning)},
		{"Unknown", strconv.Itoa(counts[Unknown]), "could not be determined", Unknown},
		{"Informational", strconv.Itoa(counts[Informational]), "worth knowing", Informational},
	})
	var rows [][]cell
	for _, f := range fs {
		rows = append(rows, []cell{lvl(f.Severity.String(), f.Severity), txt(f.Title), txt(f.Kind), txt(nz(f.Namespace)), txt(nz(f.Name)), txt(f.Description), txt(nz(f.Related))})
	}
	d.table([]col{{title: "Severity", w: .1}, {title: "Finding", w: .15, wrap: true}, {title: "Kind", w: .1, wrap: true}, {title: "Namespace", w: .1, wrap: true}, {title: "Resource", w: .16, mono: true, wrap: true}, {title: "Description", w: .27, wrap: true}, {title: "Related", w: .12, wrap: true}}, rows, "No findings: every reference resolves, every workload is available and every pod is healthy.")
}

func (d *doc) metadata(c Contents, topoPages, findings int) {
	s := d.snap
	var inc []string
	inc = append(inc, "Cluster Overview")
	if c.Health {
		inc = append(inc, "Health Summary")
	}
	if c.Topology {
		inc = append(inc, fmt.Sprintf("Topology (%d diagram page(s))", topoPages))
	}
	if c.Inventory {
		inc = append(inc, "Resource Inventory")
	}
	if c.Findings {
		inc = append(inc, fmt.Sprintf("Findings (%d)", findings))
	}
	notReadable := "none — every kind Mantis reads was readable"
	if len(s.NotReadable) > 0 {
		notReadable = strings.Join(s.NotReadable, ", ") + " (not permitted by RBAC; those resources are absent from this report)"
	}
	d.kv([][2]string{
		{"Generated", s.TakenAt.Format(time.RFC3339)},
		{"Generated by", "Mantis (mantis-report service), from mantis-engine's snapshot"},
		{"Cluster", s.Graph.Context},
		{"Cluster identifier", nz(s.Graph.ClusterID)},
		{"Kubernetes version", nz(s.Graph.Version)},
		{"Graph revision", strconv.FormatUint(s.Graph.Revision, 10)},
		{"Engine sync mode", nz(s.Graph.SyncMode)},
		{"Sections", strings.Join(inc, ", ")},
		{"Kinds not readable", notReadable},
		{"Sensitive data", "Secret values, ConfigMap values and container environment are never read into the report."},
		{"Protection", "Encrypted with AES-256 (PDF standard security handler, revision 6). The password is chosen by the requester and is not stored or logged by Mantis."},
		{"Scope", "Read-only: generating this report did not modify any Kubernetes resource."},
	})
}

// ---------------------------------------------------------------- inventory

func (d *doc) inventory() {
	s := d.snap
	d.start("inv-ns", "Namespace-wise Resource Inventory", 0)
	count := map[string]map[string]int{}
	for _, n := range s.Graph.Nodes {
		if !n.Resolved || n.Namespace == "" {
			continue
		}
		if count[n.Namespace] == nil {
			count[n.Namespace] = map[string]int{}
		}
		count[n.Namespace][n.Kind]++
		count[n.Namespace]["_total"]++
	}
	status := map[string]string{}
	for _, n := range s.Namespaces {
		status[n.Name] = string(n.Status.Phase)
	}
	var rows [][]cell
	for _, ns := range s.Graph.Namespaces {
		c := count[ns]
		if c == nil {
			c = map[string]int{}
		}
		other := c["_total"] - c["Deployment"] - c["StatefulSet"] - c["DaemonSet"] - c["Service"] - c["Pod"] - c["Job"] - c["CronJob"]
		st := status[ns]
		l := Healthy
		if st != "Active" {
			l = Warning
			if st == "" {
				st, l = "Unknown", Unknown
			}
		}
		rows = append(rows, []cell{txt(ns), lvl(st, l), txt(strconv.Itoa(c["_total"])), txt(strconv.Itoa(c["Deployment"])), txt(strconv.Itoa(c["StatefulSet"])), txt(strconv.Itoa(c["DaemonSet"])),
			txt(strconv.Itoa(c["Service"])), txt(strconv.Itoa(c["Pod"])), txt(strconv.Itoa(c["Job"])), txt(strconv.Itoa(c["CronJob"])), txt(strconv.Itoa(other))})
	}
	n := func(t string) col { return col{title: t, w: .072, align: "R"} }
	d.table([]col{{title: "Namespace", w: .2, mono: true}, {title: "Status", w: .1}, n("Total"), n("Deploy"), n("STS"), n("DS"), n("Svc"), n("Pods"), n("Jobs"), n("CronJobs"), {title: "Other", w: .052, align: "R"}}, rows, "No namespaces.")

	d.start("inv-wl", "Workload Inventory", 0)
	d.heading("Deployments")
	rsByOwner, podsByOwner := map[string][]string{}, map[string]int{}
	for i := range s.ReplicaSets {
		rs := &s.ReplicaSets[i]
		for _, o := range rs.OwnerReferences {
			rsByOwner[rs.Namespace+"/"+o.Kind+"/"+o.Name] = append(rsByOwner[rs.Namespace+"/"+o.Kind+"/"+o.Name], rs.Name)
		}
	}
	for i := range s.Pods {
		p := &s.Pods[i]
		for _, o := range p.OwnerReferences {
			podsByOwner[p.Namespace+"/"+o.Kind+"/"+o.Name]++
		}
	}
	rows = nil
	for i := range s.Deployments {
		dp := &s.Deployments[i]
		w := deploymentState(dp)
		pods := 0
		for _, rs := range rsByOwner[dp.Namespace+"/Deployment/"+dp.Name] {
			pods += podsByOwner[dp.Namespace+"/ReplicaSet/"+rs]
		}
		rows = append(rows, []cell{txt(dp.Namespace), txt(dp.Name), txt(i32(w.Desired)), txt(i32(w.Ready)), txt(i32(w.Available)), txt(strings.Join(rsByOwner[dp.Namespace+"/Deployment/"+dp.Name], ", ")), txt(strconv.Itoa(pods)), lvl(w.Level.String(), w.Level)})
	}
	d.table([]col{{title: "Namespace", w: .12}, {title: "Deployment", w: .2, mono: true}, {title: "Desired", w: .07, align: "R"}, {title: "Ready", w: .07, align: "R"}, {title: "Available", w: .08, align: "R"}, {title: "ReplicaSets", w: .26, mono: true, wrap: true}, {title: "Pods", w: .06, align: "R"}, {title: "Status", w: .14}}, rows, "No Deployments.")
	d.heading("StatefulSets")
	rows = nil
	for i := range s.StatefulSets {
		w := statefulSetState(&s.StatefulSets[i])
		rows = append(rows, []cell{txt(w.Namespace), txt(w.Name), txt(i32(w.Desired)), txt(i32(w.Ready)), txt(strconv.Itoa(podsByOwner[w.Namespace+"/StatefulSet/"+w.Name])), lvl(w.Level.String(), w.Level)})
	}
	d.table([]col{{title: "Namespace", w: .16}, {title: "StatefulSet", w: .32, mono: true}, {title: "Desired", w: .1, align: "R"}, {title: "Ready", w: .1, align: "R"}, {title: "Pods", w: .1, align: "R"}, {title: "Status", w: .22}}, rows, "No StatefulSets.")
	d.heading("DaemonSets")
	rows = nil
	for i := range s.DaemonSets {
		w := daemonSetState(&s.DaemonSets[i])
		rows = append(rows, []cell{txt(w.Namespace), txt(w.Name), txt(i32(w.Desired)), txt(i32(w.Ready)), txt(i32(w.Available)), lvl(w.Level.String(), w.Level)})
	}
	d.table([]col{{title: "Namespace", w: .16}, {title: "DaemonSet", w: .32, mono: true}, {title: "Desired", w: .1, align: "R"}, {title: "Ready", w: .1, align: "R"}, {title: "Available", w: .1, align: "R"}, {title: "Status", w: .22}}, rows, "No DaemonSets.")
	d.heading("ReplicaSets")
	rows = nil
	for i := range s.ReplicaSets {
		rs := &s.ReplicaSets[i]
		desired := int32(0)
		if rs.Spec.Replicas != nil {
			desired = *rs.Spec.Replicas
		}
		rows = append(rows, []cell{txt(rs.Namespace), txt(rs.Name), txt(ownerOf(rs.OwnerReferences)), txt(i32(desired)), txt(i32(rs.Status.ReadyReplicas))})
	}
	d.table([]col{{title: "Namespace", w: .16}, {title: "ReplicaSet", w: .34, mono: true}, {title: "Owner", w: .3, mono: true}, {title: "Desired", w: .1, align: "R"}, {title: "Ready", w: .1, align: "R"}}, rows, "No ReplicaSets.")
	d.heading("Jobs")
	rows = nil
	for i := range s.Jobs {
		j := &s.Jobs[i]
		comp := "1"
		if j.Spec.Completions != nil {
			comp = i32(*j.Spec.Completions)
		}
		l := Unknown
		state := "Running"
		switch {
		case j.Status.Failed > 0 && j.Status.Succeeded == 0 && j.Status.Active == 0:
			l, state = Critical, "Failed"
		case j.Status.Succeeded > 0 && j.Status.Active == 0:
			l, state = Healthy, "Complete"
		}
		rows = append(rows, []cell{txt(j.Namespace), txt(j.Name), txt(ownerOf(j.OwnerReferences)), txt(comp), txt(i32(j.Status.Succeeded)), txt(i32(j.Status.Failed)), lvl(state, l)})
	}
	d.table([]col{{title: "Namespace", w: .14}, {title: "Job", w: .28, mono: true}, {title: "Owner", w: .2, mono: true}, {title: "Completions", w: .1, align: "R"}, {title: "Succeeded", w: .1, align: "R"}, {title: "Failed", w: .08, align: "R"}, {title: "State", w: .1}}, rows, "No Jobs.")
	d.heading("CronJobs")
	rows = nil
	for i := range s.CronJobs {
		cj := &s.CronJobs[i]
		suspend := "no"
		if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
			suspend = "yes"
		}
		last := "never"
		if cj.Status.LastScheduleTime != nil {
			last = cj.Status.LastScheduleTime.UTC().Format("2006-01-02 15:04")
		}
		rows = append(rows, []cell{txt(cj.Namespace), txt(cj.Name), txt(cj.Spec.Schedule), txt(suspend), txt(last), txt(strconv.Itoa(len(cj.Status.Active)))})
	}
	d.table([]col{{title: "Namespace", w: .15}, {title: "CronJob", w: .3, mono: true}, {title: "Schedule", w: .15, mono: true}, {title: "Suspended", w: .1}, {title: "Last scheduled (UTC)", w: .2}, {title: "Active", w: .1, align: "R"}}, rows, "No CronJobs.")

	d.start("inv-pods", "Pod Inventory", 0)
	rows = nil
	for i := range s.Pods {
		p := &s.Pods[i]
		st := ObservePod(p)
		rows = append(rows, []cell{txt(p.Namespace), txt(p.Name), lvl(st.Phase+reasonSuffix(st.Reason), st.Level), txt(nz(st.Node)), txt(strconv.Itoa(st.Containers)), txt(strconv.Itoa(st.Ready)), txt(strconv.Itoa(int(st.Restarts))), txt(nz(st.IP)), txt(nz(st.Owner))})
	}
	d.table([]col{{title: "Namespace", w: .11}, {title: "Pod", w: .22, mono: true}, {title: "Status", w: .14}, {title: "Node", w: .13, mono: true}, {title: "Containers", w: .07, align: "R"}, {title: "Ready", w: .06, align: "R"}, {title: "Restarts", w: .07, align: "R"}, {title: "Pod IP", w: .09, mono: true}, {title: "Owner", w: .11, mono: true}}, rows, "No Pods.")

	d.start("inv-net", "Networking and Service Inventory", 0)
	d.heading("Services")
	rows = nil
	for i := range s.Services {
		sv := &s.Services[i]
		var ports []string
		for _, p := range sv.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
		var sel []string
		for k, v := range sv.Spec.Selector {
			sel = append(sel, k+"="+v)
		}
		sort.Strings(sel)
		backends := 0
		for _, e := range s.Out(sv.Namespace + "/Service/" + sv.Name) {
			if e.Type == "serves" {
				backends++
			}
		}
		rows = append(rows, []cell{txt(sv.Namespace), txt(sv.Name), txt(string(sv.Spec.Type)), txt(nz(sv.Spec.ClusterIP)), txt(nz(strings.Join(ports, ", "))), txt(nz(strings.Join(sel, ", "))), txt(strconv.Itoa(backends))})
	}
	d.table([]col{{title: "Namespace", w: .12}, {title: "Service", w: .2, mono: true}, {title: "Type", w: .09}, {title: "Cluster IP", w: .11, mono: true}, {title: "Ports", w: .14, wrap: true}, {title: "Selector", w: .24, mono: true, wrap: true}, {title: "Endpoints", w: .1, align: "R"}}, rows, "No Services.")
	d.heading("Ingresses")
	rows = nil
	for i := range s.Ingresses {
		in := &s.Ingresses[i]
		var hosts []string
		for _, r := range in.Spec.Rules {
			if r.Host != "" {
				hosts = append(hosts, r.Host)
			}
		}
		var back []string
		for _, e := range s.Out(in.Namespace + "/Ingress/" + in.Name) {
			if e.Type == "routes-to" {
				t := e.To[strings.LastIndex(e.To, "/")+1:]
				if n := s.Node(e.To); n != nil && !n.Resolved {
					t += " (not found)"
				}
				back = append(back, t)
			}
		}
		class := ""
		if in.Spec.IngressClassName != nil {
			class = *in.Spec.IngressClassName
		}
		rows = append(rows, []cell{txt(in.Namespace), txt(in.Name), txt(nz(class)), txt(nz(strings.Join(hosts, ", "))), txt(nz(strings.Join(back, ", ")))})
	}
	d.table([]col{{title: "Namespace", w: .14}, {title: "Ingress", w: .22, mono: true}, {title: "Class", w: .12}, {title: "Hosts", w: .26, wrap: true}, {title: "Routes to Services", w: .26, mono: true, wrap: true}}, rows, "No Ingresses.")
	d.heading("NetworkPolicies")
	rows = nil
	for i := range s.NetworkPolicies {
		np := &s.NetworkPolicies[i]
		var types []string
		for _, t := range np.Spec.PolicyTypes {
			types = append(types, string(t))
		}
		applies := 0
		for _, e := range s.Out(np.Namespace + "/NetworkPolicy/" + np.Name) {
			if e.Type == "applies-to" {
				applies++
			}
		}
		rows = append(rows, []cell{txt(np.Namespace), txt(np.Name), txt(nz(strings.Join(types, ", "))), txt(strconv.Itoa(len(np.Spec.Ingress))), txt(strconv.Itoa(len(np.Spec.Egress))), txt(strconv.Itoa(applies))})
	}
	d.table([]col{{title: "Namespace", w: .16}, {title: "NetworkPolicy", w: .3, mono: true}, {title: "Types", w: .16}, {title: "Ingress rules", w: .12, align: "R"}, {title: "Egress rules", w: .12, align: "R"}, {title: "Pods", w: .14, align: "R"}}, rows, "No NetworkPolicies.")

	d.start("inv-store", "Storage and Configuration Inventory", 0)
	d.heading("PersistentVolumeClaims")
	rows = nil
	for i := range s.PVCs {
		c := &s.PVCs[i]
		l := Healthy
		switch c.Status.Phase {
		case corev1.ClaimPending:
			l = Warning
		case corev1.ClaimLost:
			l = Critical
		}
		sc := ""
		if c.Spec.StorageClassName != nil {
			sc = *c.Spec.StorageClassName
		}
		capacity := ""
		if q, ok := c.Status.Capacity[corev1.ResourceStorage]; ok {
			capacity = q.String()
		}
		rows = append(rows, []cell{txt(c.Namespace), txt(c.Name), lvl(nz(string(c.Status.Phase)), l), txt(nz(capacity)), txt(modes(c.Spec.AccessModes)), txt(nz(sc)), txt(nz(c.Spec.VolumeName))})
	}
	d.table([]col{{title: "Namespace", w: .13}, {title: "Claim", w: .2, mono: true}, {title: "Status", w: .1}, {title: "Capacity", w: .09}, {title: "Access", w: .1}, {title: "StorageClass", w: .14}, {title: "Volume", w: .24, mono: true}}, rows, "No PersistentVolumeClaims.")
	d.heading("PersistentVolumes")
	rows = nil
	for i := range s.PVs {
		v := &s.PVs[i]
		claim := ""
		if v.Spec.ClaimRef != nil {
			claim = v.Spec.ClaimRef.Namespace + "/" + v.Spec.ClaimRef.Name
		}
		capacity := ""
		if q, ok := v.Spec.Capacity[corev1.ResourceStorage]; ok {
			capacity = q.String()
		}
		l := Healthy
		if v.Status.Phase == corev1.VolumeFailed {
			l = Critical
		} else if v.Status.Phase != corev1.VolumeBound {
			l = Unknown
		}
		rows = append(rows, []cell{txt(v.Name), txt(nz(capacity)), txt(string(v.Spec.PersistentVolumeReclaimPolicy)), lvl(nz(string(v.Status.Phase)), l), txt(nz(claim)), txt(nz(v.Spec.StorageClassName))})
	}
	d.table([]col{{title: "Volume", w: .26, mono: true}, {title: "Capacity", w: .1}, {title: "Reclaim", w: .1}, {title: "Status", w: .12}, {title: "Claim", w: .26, mono: true}, {title: "StorageClass", w: .16}}, rows, "No PersistentVolumes.")
	d.heading("StorageClasses")
	rows = nil
	for i := range s.StorageClasses {
		sc := &s.StorageClasses[i]
		reclaim, mode := "Delete", "Immediate"
		if sc.ReclaimPolicy != nil {
			reclaim = string(*sc.ReclaimPolicy)
		}
		if sc.VolumeBindingMode != nil {
			mode = string(*sc.VolumeBindingMode)
		}
		def := "no"
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			def = "yes"
		}
		rows = append(rows, []cell{txt(sc.Name), txt(sc.Provisioner), txt(reclaim), txt(mode), txt(def)})
	}
	d.table([]col{{title: "StorageClass", w: .22, mono: true}, {title: "Provisioner", w: .32, mono: true}, {title: "Reclaim", w: .14}, {title: "Binding mode", w: .2}, {title: "Default", w: .12}}, rows, "No StorageClasses.")
	d.heading("ConfigMaps")
	usedBy := func(id string, types ...string) int {
		n := 0
		for _, e := range s.In(id) {
			for _, t := range types {
				if e.Type == t {
					n++
				}
			}
		}
		return n
	}
	rows = nil
	for i := range s.ConfigMaps {
		c := &s.ConfigMaps[i]
		if !s.InGraph("ConfigMap", c.Namespace, c.Name) {
			continue
		}
		rows = append(rows, []cell{txt(c.Namespace), txt(c.Name), txt(strconv.Itoa(len(c.Data))), txt(strconv.Itoa(usedBy(c.Namespace+"/ConfigMap/"+c.Name, "references", "mounts")))})
	}
	d.table([]col{{title: "Namespace", w: .2}, {title: "ConfigMap", w: .44, mono: true}, {title: "Keys", w: .16, align: "R"}, {title: "Used by pods", w: .2, align: "R"}}, rows, "No ConfigMaps (system-managed ones are not listed).")
	d.heading("Secrets (metadata only)")
	d.para("Only names, types and usage are listed. Secret values are never read into this report.")
	rows = nil
	for i := range s.Secrets {
		c := &s.Secrets[i]
		if !s.InGraph("Secret", c.Namespace, c.Name) {
			continue
		}
		rows = append(rows, []cell{txt(c.Namespace), txt(c.Name), txt(string(c.Type)), txt(strconv.Itoa(usedBy(c.Namespace+"/Secret/"+c.Name, "references", "mounts")))})
	}
	d.table([]col{{title: "Namespace", w: .2}, {title: "Secret", w: .36, mono: true}, {title: "Type", w: .26}, {title: "Used by pods", w: .18, align: "R"}}, rows, "No Secrets (system-managed ones are not listed).")

	d.start("inv-other", "Other Supported Resources", 0)
	d.heading("HorizontalPodAutoscalers")
	rows = nil
	for i := range s.HPAs {
		h := &s.HPAs[i]
		min := "1"
		if h.Spec.MinReplicas != nil {
			min = i32(*h.Spec.MinReplicas)
		}
		rows = append(rows, []cell{txt(h.Namespace), txt(h.Name), txt(h.Spec.ScaleTargetRef.Kind + "/" + h.Spec.ScaleTargetRef.Name), txt(min), txt(i32(h.Spec.MaxReplicas)), txt(i32(h.Status.CurrentReplicas))})
	}
	d.table([]col{{title: "Namespace", w: .15}, {title: "HPA", w: .25, mono: true}, {title: "Target", w: .3, mono: true}, {title: "Min", w: .1, align: "R"}, {title: "Max", w: .1, align: "R"}, {title: "Current", w: .1, align: "R"}}, rows, "No HorizontalPodAutoscalers.")
	d.heading("ServiceAccounts")
	rows = nil
	for i := range s.ServiceAccounts {
		a := &s.ServiceAccounts[i]
		if !s.InGraph("ServiceAccount", a.Namespace, a.Name) {
			continue
		}
		id := a.Namespace + "/ServiceAccount/" + a.Name
		rows = append(rows, []cell{txt(a.Namespace), txt(a.Name), txt(strconv.Itoa(usedBy(id, "uses-service-account"))), txt(strconv.Itoa(usedBy(id, "grants-to")))})
	}
	d.table([]col{{title: "Namespace", w: .22}, {title: "ServiceAccount", w: .42, mono: true}, {title: "Used by pods", w: .18, align: "R"}, {title: "Bindings", w: .18, align: "R"}}, rows, "No ServiceAccounts (the default account in each namespace is not listed).")
	d.heading("Roles and RoleBindings")
	rows = nil
	for i := range s.Roles {
		r := &s.Roles[i]
		if s.InGraph("Role", r.Namespace, r.Name) {
			rows = append(rows, []cell{txt("Role"), txt(r.Namespace), txt(r.Name), txt(fmt.Sprintf("%d rules", len(r.Rules)))})
		}
	}
	for i := range s.RoleBindings {
		b := &s.RoleBindings[i]
		if s.InGraph("RoleBinding", b.Namespace, b.Name) {
			rows = append(rows, []cell{txt("RoleBinding"), txt(b.Namespace), txt(b.Name), txt(fmt.Sprintf("%s/%s · %d subjects", b.RoleRef.Kind, b.RoleRef.Name, len(b.Subjects)))})
		}
	}
	for i := range s.ClusterRoles {
		r := &s.ClusterRoles[i]
		if s.InGraph("ClusterRole", "", r.Name) {
			rows = append(rows, []cell{txt("ClusterRole"), txt("—"), txt(r.Name), txt(fmt.Sprintf("%d rules", len(r.Rules)))})
		}
	}
	for i := range s.ClusterRoleBindings {
		b := &s.ClusterRoleBindings[i]
		if s.InGraph("ClusterRoleBinding", "", b.Name) {
			rows = append(rows, []cell{txt("ClusterRoleBinding"), txt("—"), txt(b.Name), txt(fmt.Sprintf("%s/%s · %d subjects", b.RoleRef.Kind, b.RoleRef.Name, len(b.Subjects)))})
		}
	}
	d.table([]col{{title: "Kind", w: .16}, {title: "Namespace", w: .16}, {title: "Name", w: .34, mono: true}, {title: "Details", w: .34, wrap: true}}, rows, "No RBAC objects (control-plane system: objects are not listed).")
	d.heading("ResourceQuotas and LimitRanges")
	rows = nil
	for i := range s.ResourceQuotas {
		q := &s.ResourceQuotas[i]
		rows = append(rows, []cell{txt("ResourceQuota"), txt(q.Namespace), txt(q.Name), txt(fmt.Sprintf("%d limits", len(q.Spec.Hard)))})
	}
	for i := range s.LimitRanges {
		l := &s.LimitRanges[i]
		rows = append(rows, []cell{txt("LimitRange"), txt(l.Namespace), txt(l.Name), txt(fmt.Sprintf("%d entries", len(l.Spec.Limits)))})
	}
	d.table([]col{{title: "Kind", w: .16}, {title: "Namespace", w: .16}, {title: "Name", w: .34, mono: true}, {title: "Details", w: .34}}, rows, "No ResourceQuotas or LimitRanges.")
}

func ownerOf(refs []metav1.OwnerReference) string {
	if len(refs) == 0 {
		return "—"
	}
	return refs[0].Kind + "/" + refs[0].Name
}

func modes(m []corev1.PersistentVolumeAccessMode) string {
	short := map[corev1.PersistentVolumeAccessMode]string{corev1.ReadWriteOnce: "RWO", corev1.ReadOnlyMany: "ROX", corev1.ReadWriteMany: "RWX", corev1.ReadWriteOncePod: "RWOP"}
	var out []string
	for _, x := range m {
		out = append(out, short[x])
	}
	return nz(strings.Join(out, ", "))
}

func nz(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func i32(v int32) string { return strconv.Itoa(int(v)) }

// ---------------------------------------------------------------- topology

// topologyPage draws one validated diagram on a page sized to the graph.
func (d *doc) topologyPage(p *page, idx, total int) {
	pdf := d.pdf
	bw, bh := p.Bounds[2]-p.Bounds[0], p.Bounds[3]-p.Bounds[1]
	const headerH, legendH, side = 86.0, 74.0, 30.0
	s := baseScale
	s = math.Min(s, (maxPageDim-2*side)/bw)
	s = math.Min(s, (maxPageDim-headerH-legendH)/bh)
	w := math.Max(pageW, bw*s+2*side)
	h := math.Max(pageH, bh*s+headerH+legendH)
	d.section = "Topology"
	pdf.AddPageFormat("P", fpdf.SizeType{Wd: w, Ht: h})
	if idx == 0 {
		d.mark("topology", "Complete Cluster Topology", 0)
	}
	d.mark("topo:"+strconv.Itoa(idx), p.Title, 1)

	// Title band.
	pdf.SetFont("Helvetica", "B", 16)
	d.text(cText)
	pdf.Text(side, 58, d.tr(fmt.Sprintf("Topology · %s", p.Title)))
	pdf.SetFont("Helvetica", "", 8.5)
	d.text(cMuted)
	shown := fmt.Sprintf("%d resources drawn individually", p.drawn)
	if p.inFolded > 0 {
		shown += fmt.Sprintf(", %d in folded groups", p.inFolded)
	}
	if p.inGroups > 0 {
		shown += fmt.Sprintf(", %d inside collapsed namespaces", p.inGroups)
	}
	pdf.Text(side, 74, d.tr(fmt.Sprintf("Diagram %d of %d · %s · %d lines · drawn at %d%% · graph revision %d", idx+1, total, shown, len(p.edges), int(math.Round(s/baseScale*100)), d.snap.Graph.Revision)))

	tx := func(x float64) float64 { return side + (x-p.Bounds[0])*s }
	ty := func(y float64) float64 { return headerH + (y-p.Bounds[1])*s }

	// Groups.
	for i, g := range p.Groups {
		if g.Collapsed {
			continue
		}
		c := nsColor(g.Key, i)
		pdf.SetAlpha(0.05, "Normal")
		d.color(true, c)
		pdf.RoundedRect(tx(g.X0), ty(g.Y0), (g.X1-g.X0)*s, (g.Y1-g.Y0)*s, 10*s, "1234", "F")
		pdf.SetAlpha(1, "Normal")
		d.color(false, blend(c, 0.45))
		pdf.SetLineWidth(math.Max(0.5, 1*s))
		if g.Key == "infra" {
			pdf.SetDashPattern([]float64{6 * s, 5 * s}, 0)
		}
		pdf.RoundedRect(tx(g.X0), ty(g.Y0), (g.X1-g.X0)*s, (g.Y1-g.Y0)*s, 10*s, "1234", "D")
		pdf.SetDashPattern([]float64{}, 0)
		label := strings.TrimPrefix(g.Key, "ns:")
		kind := "NAMESPACE  "
		if g.Key == "infra" {
			label, kind = "cluster infrastructure", ""
		}
		pdf.SetFont("Helvetica", "B", 10*s/baseScale*0.9)
		d.text(c)
		pdf.Text(tx(g.X0)+12*s, ty(g.Y0)+21*s, d.tr(kind))
		kw := pdf.GetStringWidth(d.tr(kind))
		pdf.SetFont("Helvetica", "B", 13*s)
		d.text(cText)
		pdf.Text(tx(g.X0)+12*s+kw, ty(g.Y0)+21*s, d.tr(label))
	}

	// Edges.
	pdf.SetLineCapStyle("round")
	pdf.SetLineJoinStyle("round")
	var taken [][2]float64
	for _, e := range p.edges {
		c := relColor[e.category]
		if e.ghost {
			c = levelColor[Critical]
		}
		lw := 1.2 * s
		if e.category == "ownership" {
			lw = 1.7 * s
		}
		pdf.SetLineWidth(math.Max(lw, 0.35))
		d.color(false, c)
		if dash, ok := relDash[e.category]; ok || e.ghost {
			if e.ghost {
				dash = []float64{4, 3}
			}
			sd := make([]float64, len(dash))
			for i, v := range dash {
				sd[i] = v * s
			}
			pdf.SetDashPattern(sd, 0)
		}
		pts := e.Points
		if e.Cross {
			for i := 1; i < len(pts); i++ {
				pdf.Line(tx(pts[i-1][0]), ty(pts[i-1][1]), tx(pts[i][0]), ty(pts[i][1]))
			}
		} else {
			for i := 1; i < len(pts); i++ {
				a, b := pts[i-1], pts[i]
				dx := (b[0] - a[0]) / 2
				pdf.CurveBezierCubic(tx(a[0]), ty(a[1]), tx(a[0]+dx), ty(a[1]), tx(b[0]-dx), ty(b[1]), tx(b[0]), ty(b[1]), "D")
			}
		}
		pdf.SetDashPattern([]float64{}, 0)
		// Arrowhead at the target end.
		last, prev := pts[len(pts)-1], pts[len(pts)-2]
		ang := math.Atan2(last[1]-prev[1], last[0]-prev[0])
		if !e.Cross {
			ang = 0
			if last[0] < prev[0] {
				ang = math.Pi
			}
		}
		al := 6 * s
		x, y := tx(last[0]), ty(last[1])
		d.color(true, c)
		pdf.Polygon([]fpdf.PointType{{X: x, Y: y}, {X: x - al*math.Cos(ang-0.45), Y: y - al*math.Sin(ang-0.45)}, {X: x - al*math.Cos(ang+0.45), Y: y - al*math.Sin(ang+0.45)}}, "F")
		if e.Cross && len(e.Keys) > 1 {
			// Same placement rule as the app: the longest free segment,
			// never on top of another count.
			if lx, ly, ok := placeLabel(pts, &taken); ok {
				pdf.SetFont("Helvetica", "B", 9*s)
				d.text(c)
				label := fmt.Sprintf("%d links", len(e.Keys))
				pdf.Text(tx(lx)-pdf.GetStringWidth(label)/2, ty(ly), label)
			}
		}
	}
	pdf.SetLineCapStyle("butt")

	// Cards.
	ids := make([]string, 0, len(p.nodes))
	for id := range p.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		d.card(p.nodes[id], s, tx, ty)
	}

	// Legend and continuation references.
	ly := h - legendH + 14
	x := side
	pdf.SetFont("Helvetica", "B", 8)
	d.text(cMuted)
	pdf.Text(x, ly, "RELATIONSHIPS")
	x += 78
	pdf.SetFont("Helvetica", "", 8)
	for _, r := range []struct{ cat, label string }{{"ownership", "owns"}, {"networking", "selects / serves / routes"}, {"configuration", "config / secret / identity"},
		{"storage", "storage"}, {"scheduling", "runs on"}, {"security", "RBAC"}, {"autoscaling", "scales"}} {
		d.color(false, relColor[r.cat])
		pdf.SetLineWidth(1.4)
		if dash, ok := relDash[r.cat]; ok {
			pdf.SetDashPattern(dash, 0)
		}
		pdf.Line(x, ly-3, x+18, ly-3)
		pdf.SetDashPattern([]float64{}, 0)
		d.text(cText)
		pdf.Text(x+22, ly, r.label)
		x += 30 + pdf.GetStringWidth(r.label)
	}
	d.color(false, levelColor[Critical])
	pdf.SetDashPattern([]float64{4, 3}, 0)
	pdf.Line(x, ly-3, x+18, ly-3)
	pdf.SetDashPattern([]float64{}, 0)
	pdf.Text(x+22, ly, "points at something missing")
	pdf.SetFont("Helvetica", "", 7.8)
	d.text(cMuted)
	pdf.Text(side, ly+14, d.tr("Lines show relationships the Kubernetes API declares (ownership, spec references, label selectors, EndpointSlice records); they are not observed traffic. A card's \"-> N\" line summarizes many relationships of one kind; every relationship is listed in the inventory."))
	if len(p.external) > 0 {
		nss := make([]string, 0, len(p.external))
		for ns := range p.external {
			nss = append(nss, ns)
		}
		sort.Strings(nss)
		var refs []string
		for _, ns := range nss {
			ref := fmt.Sprintf("%s (%d)", ns, p.external[ns])
			for j, op := range d.pagesIndex {
				if op == ns {
					ref = fmt.Sprintf("%s (%d, page {p:topo:%d})", ns, p.external[ns], j)
				}
			}
			refs = append(refs, ref)
		}
		pdf.SetFont("Helvetica", "B", 8)
		d.text(cText)
		pdf.Text(side, ly+28, d.tr("Continues in other namespaces: "+strings.Join(refs, ", ")))
	}
}

// card draws one resource, aggregate or collapsed-namespace card.
func (d *doc) card(v *vnode, s float64, tx, ty func(float64) float64) {
	pdf := d.pdf
	x, y, w, h := tx(v.X-v.W/2), ty(v.Y-v.H/2), v.W*s, v.H*s
	r := 8 * s
	switch v.Type {
	case "groupcard":
		d.color(true, cWhite)
		d.color(false, cBorder)
		pdf.SetLineWidth(0.8 * s)
		pdf.RoundedRect(x, y, w, h, 11*s, "1234", "FD")
		pdf.SetFont("Helvetica", "B", 9*s)
		d.text(cMuted)
		kind := "NAMESPACE"
		if v.Group == "infra" {
			kind = "CLUSTER-SCOPED"
		}
		pdf.Text(x+14*s, y+20*s, kind)
		pdf.SetFont("Helvetica", "B", 15*s)
		d.text(cText)
		pdf.Text(x+14*s, y+41*s, d.fitAt(v.groupLabel, w-28*s))
		pdf.SetFont("Helvetica", "", 11*s)
		d.text(cMuted)
		pdf.Text(x+14*s, y+61*s, d.tr(fmt.Sprintf("%d resource%s · %d pod%s", v.total, plS(v.total), v.pods, plS(v.pods))))
		var problems []string
		if v.bad > 0 {
			problems = append(problems, fmt.Sprintf("%d failing", v.bad))
		}
		if v.broken > 0 {
			problems = append(problems, fmt.Sprintf("%d broken reference%s", v.broken, plS(v.broken)))
		}
		if len(problems) > 0 {
			d.text(levelColor[Critical])
			pdf.Text(x+14*s, y+78*s, d.tr(strings.Join(problems, " · ")))
		}
		return
	case "aggregate":
		d.color(true, cSurf)
		d.color(false, cBorder)
		pdf.SetLineWidth(0.8 * s)
		pdf.RoundedRect(x+6*s, y-5*s, w-12*s, h, r, "1234", "FD")
	}
	d.color(true, cWhite)
	if v.Type == "resource" && !v.resolved {
		d.color(true, rgb{254, 242, 242})
	}
	border := rgb{203, 213, 225}
	pdf.SetLineWidth(math.Max(0.4, 0.9*s))
	if v.Type == "resource" && !v.resolved {
		border = levelColor[Critical]
		if v.unverified {
			border = cMuted
		}
		pdf.SetDashPattern([]float64{4 * s, 3 * s}, 0)
	}
	d.color(false, border)
	pdf.RoundedRect(x, y, w, h, r, "1234", "FD")
	pdf.SetDashPattern([]float64{}, 0)
	cat := kindCat[v.kind]
	if cat == "" {
		cat = "other"
	}
	d.color(true, catColor[cat])
	pdf.Rect(x, y+1*s, 4*s, h-2*s, "F")
	pdf.SetFont("Helvetica", "B", 7.5*s/baseScale)
	d.text(catColor[cat])
	kind := strings.ToUpper(v.kind)
	if v.Type == "aggregate" {
		kind = "GROUP OF " + strings.ToUpper(plural(v.kind))
	}
	pdf.Text(x+14*s, y+16*s, d.fitAt(kind, w-40*s))
	pdf.SetFont("Courier", "B", 12*s)
	d.text(cText)
	name := v.name
	if v.Type == "aggregate" {
		name = v.label
	}
	pdf.Text(x+14*s, y+32*s, d.fitAt(name, w-24*s))
	if v.Type == "resource" {
		switch {
		case !v.resolved:
			pdf.SetFont("Helvetica", "B", 6.5*s/baseScale)
			flag := "NOT FOUND"
			c := levelColor[Critical]
			if v.unverified {
				flag, c = "UNVERIFIED", cMuted
			}
			d.text(c)
			pdf.Text(x+w-10*s-pdf.GetStringWidth(flag), y+15*s, flag)
		case v.status != "":
			l := map[string]Level{"ok": Healthy, "crit": Critical, "warn": Warning}[v.status]
			d.color(true, levelColor[l])
			pdf.Circle(x+w-12*s, y+12*s, 4*s, "F")
		}
	} else if v.bad > 0 {
		pdf.SetFont("Helvetica", "B", 6.5*s/baseScale)
		d.text(levelColor[Critical])
		f := fmt.Sprintf("%d FAILING", v.bad)
		pdf.Text(x+w-10*s-pdf.GetStringWidth(f), y+15*s, f)
	}
	if len(v.Badges) > 0 && h > 50*s {
		pdf.SetFont("Helvetica", "", 10*s)
		d.text(cMuted)
		pdf.Text(x+14*s, y+48*s, d.fitAt(strings.ReplaceAll(strings.Join(v.Badges, "  ·  "), "→", "->"), w-26*s))
	}
}

func (d *doc) fitAt(t string, w float64) string { return d.fit(t, w) }

func nsColor(key string, i int) rgb {
	if key == "infra" {
		return cMuted
	}
	palette := []rgb{{79, 138, 28}, {47, 111, 224}, {162, 63, 209}, {160, 100, 0}, {10, 143, 124}, {184, 92, 23}, {194, 63, 130}, {31, 143, 78}, {122, 79, 209}, {15, 143, 130}}
	h := uint32(0)
	for _, c := range key {
		h = h*31 + uint32(c)
	}
	return palette[h%uint32(len(palette))]
}

func blend(c rgb, a float64) rgb {
	return rgb{int(float64(c[0])*a + 255*(1-a)), int(float64(c[1])*a + 255*(1-a)), int(float64(c[2])*a + 255*(1-a))}
}

// placeLabel finds a spot for a bundle's count on one of its three longest
// segments that is clear of every label already placed (layout px).
func placeLabel(pts [][2]float64, taken *[][2]float64) (float64, float64, bool) {
	type seg struct {
		a, b [2]float64
		l    float64
	}
	var segs []seg
	for i := 1; i < len(pts); i++ {
		segs = append(segs, seg{pts[i-1], pts[i], math.Hypot(pts[i][0]-pts[i-1][0], pts[i][1]-pts[i-1][1])})
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].l > segs[j].l })
	if len(segs) > 3 {
		segs = segs[:3]
	}
	for _, sg := range segs {
		for _, f := range []float64{0.5, 0.3, 0.7} {
			x, y := sg.a[0]+(sg.b[0]-sg.a[0])*f, sg.a[1]+(sg.b[1]-sg.a[1])*f-6
			free := true
			for _, q := range *taken {
				if math.Abs(q[0]-x) < 56 && math.Abs(q[1]-y) < 16 {
					free = false
					break
				}
			}
			if free {
				*taken = append(*taken, [2]float64{x, y})
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func plS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
