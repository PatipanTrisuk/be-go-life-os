package service

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"go-be/internal/assets"
	"go-be/internal/models"
)

type habitDashboardPDFCopy struct {
	Title           string
	Generated       string
	Summary         string
	Active          string
	Paused          string
	Archived        string
	CompletedToday  string
	PendingToday    string
	AvgStreak       string
	BestStreak      string
	ByStatus        string
	ByFrequency     string
	ByCategory      string
	CheckinTrend    string
	StreakBuckets   string
	Date            string
	Count           string
	Key             string
	Uncategorized   string
	StatusActive    string
	StatusPaused    string
	StatusArchived  string
	FrequencyDaily  string
	FrequencyWeekly string
	Empty           string
}

func habitDashboardPDFLabels(locale string) habitDashboardPDFCopy {
	if strings.EqualFold(locale, "th") {
		return habitDashboardPDFCopy{
			Title:           "รายงานแดชบอร์ดนิสัย",
			Generated:       "สร้างเมื่อ",
			Summary:         "สรุปภาพรวม",
			Active:          "ใช้งาน",
			Paused:          "พักชั่วคราว",
			Archived:        "เก็บถาวร",
			CompletedToday:  "ทำแล้ววันนี้",
			PendingToday:    "ค้างวันนี้",
			AvgStreak:       "Streak เฉลี่ย",
			BestStreak:      "Streak สูงสุด",
			ByStatus:        "ตามสถานะ",
			ByFrequency:     "ตามความถี่",
			ByCategory:      "หมวดหมู่ยอดนิยม",
			CheckinTrend:    "เช็คอินรายวัน (14 วันล่าสุด)",
			StreakBuckets:   "การกระจาย Streak",
			Date:            "วันที่",
			Count:           "จำนวน",
			Key:             "รายการ",
			Uncategorized:   "ไม่มีหมวดหมู่",
			StatusActive:    "ใช้งาน",
			StatusPaused:    "พักชั่วคราว",
			StatusArchived:  "เก็บถาวร",
			FrequencyDaily:  "รายวัน",
			FrequencyWeekly: "รายสัปดาห์",
			Empty:           "ยังไม่มีข้อมูลนิสัย",
		}
	}
	return habitDashboardPDFCopy{
		Title:           "Habits dashboard report",
		Generated:       "Generated",
		Summary:         "Summary",
		Active:          "Active",
		Paused:          "Paused",
		Archived:        "Archived",
		CompletedToday:  "Done today",
		PendingToday:    "Pending today",
		AvgStreak:       "Avg streak",
		BestStreak:      "Best streak",
		ByStatus:        "By status",
		ByFrequency:     "By frequency",
		ByCategory:      "Top categories",
		CheckinTrend:    "Daily check-ins (last 14 days)",
		StreakBuckets:   "Streak distribution",
		Date:            "Date",
		Count:           "Count",
		Key:             "Item",
		Uncategorized:   "Uncategorized",
		StatusActive:    "Active",
		StatusPaused:    "Paused",
		StatusArchived:  "Archived",
		FrequencyDaily:  "Daily",
		FrequencyWeekly: "Weekly",
		Empty:           "No habits data yet",
	}
}

func (s *HabitService) BuildDashboardPDF(dashboard *models.HabitDashboard, locale string) ([]byte, string, error) {
	regular, err := assets.SarabunRegular()
	if err != nil {
		return nil, "", err
	}
	bold, err := assets.SarabunBold()
	if err != nil {
		return nil, "", err
	}

	copy := habitDashboardPDFLabels(locale)
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AddUTF8FontFromBytes("Sarabun", "", regular)
	pdf.AddUTF8FontFromBytes("Sarabun", "B", bold)
	pdf.AddPage()

	now := time.Now()
	fileName := fmt.Sprintf("habits-dashboard-%s.pdf", now.Format("2006-01-02"))

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
		case "active":
			return copy.StatusActive
		case "paused":
			return copy.StatusPaused
		case "archived":
			return copy.StatusArchived
		default:
			return key
		}
	}
	frequencyLabel := func(key string) string {
		switch key {
		case "daily":
			return copy.FrequencyDaily
		case "weekly":
			return copy.FrequencyWeekly
		default:
			return key
		}
	}

	writeSectionTitle(copy.Summary)
	writeKVTable([2]string{copy.Key, copy.Count}, [][2]string{
		{copy.Active, fmt.Sprintf("%d", dashboard.Stats.Active)},
		{copy.Paused, fmt.Sprintf("%d", dashboard.Stats.Paused)},
		{copy.Archived, fmt.Sprintf("%d", dashboard.Stats.Archived)},
		{copy.CompletedToday, fmt.Sprintf("%d", dashboard.Stats.CompletedToday)},
		{copy.PendingToday, fmt.Sprintf("%d", dashboard.Stats.PendingToday)},
		{copy.AvgStreak, fmt.Sprintf("%d", dashboard.Stats.AvgStreak)},
		{copy.BestStreak, fmt.Sprintf("%d", dashboard.Stats.BestStreak)},
	})

	statusRows := make([][2]string, 0, len(dashboard.ByStatus))
	for _, item := range dashboard.ByStatus {
		statusRows = append(statusRows, [2]string{statusLabel(item.Key), fmt.Sprintf("%d", item.Count)})
	}
	writeSectionTitle(copy.ByStatus)
	writeKVTable([2]string{copy.Key, copy.Count}, statusRows)

	freqRows := make([][2]string, 0, len(dashboard.ByFrequency))
	for _, item := range dashboard.ByFrequency {
		freqRows = append(freqRows, [2]string{frequencyLabel(item.Key), fmt.Sprintf("%d", item.Count)})
	}
	writeSectionTitle(copy.ByFrequency)
	writeKVTable([2]string{copy.Key, copy.Count}, freqRows)

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

	writeSectionTitle(copy.CheckinTrend)
	colW := []float64{70, 40}
	pdf.SetFont("Sarabun", "B", 11)
	pdf.SetFillColor(245, 245, 245)
	pdf.CellFormat(colW[0], 8, copy.Date, "1", 0, "L", true, 0, "")
	pdf.CellFormat(colW[1], 8, copy.Count, "1", 1, "R", true, 0, "")
	pdf.SetFont("Sarabun", "", 11)
	for _, item := range dashboard.CheckinTrend {
		pdf.CellFormat(colW[0], 7, item.Date, "1", 0, "L", false, 0, "")
		pdf.CellFormat(colW[1], 7, fmt.Sprintf("%d", item.Count), "1", 1, "R", false, 0, "")
	}

	streakRows := make([][2]string, 0, len(dashboard.StreakBuckets))
	for _, item := range dashboard.StreakBuckets {
		streakRows = append(streakRows, [2]string{item.Bucket, fmt.Sprintf("%d", item.Count)})
	}
	writeSectionTitle(copy.StreakBuckets)
	writeKVTable([2]string{copy.Key, copy.Count}, streakRows)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), fileName, nil
}

func (s *HabitService) ExportDashboardPDF(ctx context.Context, userID, locale string) ([]byte, string, error) {
	dashboard, err := s.Dashboard(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	return s.BuildDashboardPDF(dashboard, locale)
}
