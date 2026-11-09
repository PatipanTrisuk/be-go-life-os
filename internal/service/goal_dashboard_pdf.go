package service

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"go-be/internal/assets"
	"go-be/internal/models"
)

type dashboardPDFCopy struct {
	Title           string
	Generated       string
	Summary         string
	Total           string
	Active          string
	Completed       string
	Overdue         string
	AvgProgress     string
	ByStatus        string
	ByPriority      string
	ByCategory      string
	Trend           string
	ProgressBuckets string
	Month           string
	Created         string
	CompletedCol    string
	Count           string
	Key             string
	Uncategorized   string
	StatusDraft     string
	StatusActive    string
	StatusCompleted string
	StatusCancelled string
	StatusArchived  string
	PriorityLow     string
	PriorityMedium  string
	PriorityHigh    string
	Empty           string
}

func dashboardPDFLabels(locale string) dashboardPDFCopy {
	if strings.EqualFold(locale, "th") {
		return dashboardPDFCopy{
			Title:           "รายงานแดชบอร์ดเป้าหมาย",
			Generated:       "สร้างเมื่อ",
			Summary:         "สรุปภาพรวม",
			Total:           "ทั้งหมด",
			Active:          "กำลังทำ",
			Completed:       "สำเร็จ",
			Overdue:         "เกินกำหนด",
			AvgProgress:     "ความคืบหน้าเฉลี่ย",
			ByStatus:        "ตามสถานะ",
			ByPriority:      "ตามความสำคัญ",
			ByCategory:      "หมวดหมู่ยอดนิยม",
			Trend:           "แนวโน้ม 6 เดือน (สร้าง / สำเร็จ)",
			ProgressBuckets: "การกระจายความคืบหน้า",
			Month:           "เดือน",
			Created:         "สร้าง",
			CompletedCol:    "สำเร็จ",
			Count:           "จำนวน",
			Key:             "รายการ",
			Uncategorized:   "ไม่มีหมวดหมู่",
			StatusDraft:     "ร่าง",
			StatusActive:    "กำลังทำ",
			StatusCompleted: "สำเร็จ",
			StatusCancelled: "ยกเลิก",
			StatusArchived:  "เก็บถาวร",
			PriorityLow:     "ต่ำ",
			PriorityMedium:  "ปานกลาง",
			PriorityHigh:    "สูง",
			Empty:           "ยังไม่มีข้อมูลเป้าหมาย",
		}
	}
	return dashboardPDFCopy{
		Title:           "Goals dashboard report",
		Generated:       "Generated",
		Summary:         "Summary",
		Total:           "Total",
		Active:          "Active",
		Completed:       "Completed",
		Overdue:         "Overdue",
		AvgProgress:     "Avg progress",
		ByStatus:        "By status",
		ByPriority:      "By priority",
		ByCategory:      "Top categories",
		Trend:           "6-month trend (created / completed)",
		ProgressBuckets: "Progress distribution",
		Month:           "Month",
		Created:         "Created",
		CompletedCol:    "Completed",
		Count:           "Count",
		Key:             "Item",
		Uncategorized:   "Uncategorized",
		StatusDraft:     "Draft",
		StatusActive:    "Active",
		StatusCompleted: "Completed",
		StatusCancelled: "Cancelled",
		StatusArchived:  "Archived",
		PriorityLow:     "Low",
		PriorityMedium:  "Medium",
		PriorityHigh:    "High",
		Empty:           "No goals data yet",
	}
}

func (s *GoalService) BuildDashboardPDF(dashboard *models.GoalDashboard, locale string) ([]byte, string, error) {
	regular, err := assets.SarabunRegular()
	if err != nil {
		return nil, "", err
	}
	bold, err := assets.SarabunBold()
	if err != nil {
		return nil, "", err
	}

	copy := dashboardPDFLabels(locale)
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AddUTF8FontFromBytes("Sarabun", "", regular)
	pdf.AddUTF8FontFromBytes("Sarabun", "B", bold)
	pdf.AddPage()

	now := time.Now()
	fileName := fmt.Sprintf("goals-dashboard-%s.pdf", now.Format("2006-01-02"))

	pdf.SetFont("Sarabun", "B", 18)
	pdf.CellFormat(0, 10, copy.Title, "", 1, "L", false, 0, "")
	pdf.SetFont("Sarabun", "", 11)
	pdf.SetTextColor(90, 90, 90)
	pdf.CellFormat(0, 7, fmt.Sprintf("%s: %s", copy.Generated, now.Format("2006-01-02 15:04")), "", 1, "L", false, 0, "")
	pdf.Ln(4)
	pdf.SetTextColor(0, 0, 0)

	if dashboard.Stats.Total == 0 {
		pdf.SetFont("Sarabun", "", 12)
		pdf.MultiCell(0, 7, copy.Empty, "", "L", false)
		var buf bytes.Buffer
		if err := pdf.Output(&buf); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), fileName, nil
	}

	writeSectionTitle := func(title string) {
		pdf.Ln(2)
		pdf.SetFont("Sarabun", "B", 13)
		pdf.CellFormat(0, 8, title, "", 1, "L", false, 0, "")
		pdf.SetFont("Sarabun", "", 11)
	}

	writeKVTable := func(headers [2]string, rows [][2]string) {
		colW := []float64{90, 40}
		pdf.SetFont("Sarabun", "B", 11)
		pdf.SetFillColor(245, 245, 245)
		pdf.CellFormat(colW[0], 8, headers[0], "1", 0, "L", true, 0, "")
		pdf.CellFormat(colW[1], 8, headers[1], "1", 1, "R", true, 0, "")
		pdf.SetFont("Sarabun", "", 11)
		for _, row := range rows {
			pdf.CellFormat(colW[0], 7, row[0], "1", 0, "L", false, 0, "")
			pdf.CellFormat(colW[1], 7, row[1], "1", 1, "R", false, 0, "")
		}
	}

	statusLabel := func(key string) string {
		switch key {
		case "draft":
			return copy.StatusDraft
		case "active":
			return copy.StatusActive
		case "completed":
			return copy.StatusCompleted
		case "cancelled":
			return copy.StatusCancelled
		case "archived":
			return copy.StatusArchived
		default:
			return key
		}
	}
	priorityLabel := func(key string) string {
		switch key {
		case "low":
			return copy.PriorityLow
		case "medium":
			return copy.PriorityMedium
		case "high":
			return copy.PriorityHigh
		default:
			return key
		}
	}

	writeSectionTitle(copy.Summary)
	writeKVTable([2]string{copy.Key, copy.Count}, [][2]string{
		{copy.Total, fmt.Sprintf("%d", dashboard.Stats.Total)},
		{copy.Active, fmt.Sprintf("%d", dashboard.Stats.Active)},
		{copy.Completed, fmt.Sprintf("%d", dashboard.Stats.Completed)},
		{copy.Overdue, fmt.Sprintf("%d", dashboard.Stats.Overdue)},
		{copy.AvgProgress, fmt.Sprintf("%d%%", dashboard.Stats.AvgProgress)},
	})

	statusRows := make([][2]string, 0, len(dashboard.ByStatus))
	for _, item := range dashboard.ByStatus {
		statusRows = append(statusRows, [2]string{statusLabel(item.Key), fmt.Sprintf("%d", item.Count)})
	}
	writeSectionTitle(copy.ByStatus)
	writeKVTable([2]string{copy.Key, copy.Count}, statusRows)

	priorityRows := make([][2]string, 0, len(dashboard.ByPriority))
	for _, item := range dashboard.ByPriority {
		priorityRows = append(priorityRows, [2]string{priorityLabel(item.Key), fmt.Sprintf("%d", item.Count)})
	}
	writeSectionTitle(copy.ByPriority)
	writeKVTable([2]string{copy.Key, copy.Count}, priorityRows)

	categoryRows := make([][2]string, 0, len(dashboard.ByCategory))
	for _, item := range dashboard.ByCategory {
		label := item.Key
		if label == "uncategorized" || strings.TrimSpace(label) == "" {
			label = copy.Uncategorized
		}
		categoryRows = append(categoryRows, [2]string{label, fmt.Sprintf("%d", item.Count)})
	}
	writeSectionTitle(copy.ByCategory)
	if len(categoryRows) == 0 {
		pdf.CellFormat(0, 7, "—", "", 1, "L", false, 0, "")
	} else {
		writeKVTable([2]string{copy.Key, copy.Count}, categoryRows)
	}

	writeSectionTitle(copy.Trend)
	colW := []float64{50, 40, 40}
	pdf.SetFont("Sarabun", "B", 11)
	pdf.SetFillColor(245, 245, 245)
	pdf.CellFormat(colW[0], 8, copy.Month, "1", 0, "L", true, 0, "")
	pdf.CellFormat(colW[1], 8, copy.Created, "1", 0, "R", true, 0, "")
	pdf.CellFormat(colW[2], 8, copy.CompletedCol, "1", 1, "R", true, 0, "")
	pdf.SetFont("Sarabun", "", 11)
	for _, item := range dashboard.CreatedTrend {
		pdf.CellFormat(colW[0], 7, item.Month, "1", 0, "L", false, 0, "")
		pdf.CellFormat(colW[1], 7, fmt.Sprintf("%d", item.Created), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[2], 7, fmt.Sprintf("%d", item.Completed), "1", 1, "R", false, 0, "")
	}

	progressRows := make([][2]string, 0, len(dashboard.ProgressBuckets))
	for _, item := range dashboard.ProgressBuckets {
		progressRows = append(progressRows, [2]string{item.Bucket, fmt.Sprintf("%d", item.Count)})
	}
	writeSectionTitle(copy.ProgressBuckets)
	writeKVTable([2]string{copy.Key, copy.Count}, progressRows)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), fileName, nil
}
