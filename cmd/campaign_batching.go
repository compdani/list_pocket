package main

import (
	"strings"

	"github.com/compdani/list_pocket/models"
)

type campaignBatchingReq struct {
	Enabled     bool     `json:"enabled"`
	BatchSize   int      `json:"batch_size"`
	RepeatValue int      `json:"repeat_value"`
	RepeatUnit  string   `json:"repeat_unit"`
	Days        []string `json:"days"`
	StartTime   string   `json:"start_time"`
	EndTime     string   `json:"end_time"`
	Timezone    string   `json:"timezone"`
}

func (r campaignBatchingReq) toModel() models.CampaignBatching {
	return models.CampaignBatching{
		Enabled:     r.Enabled,
		BatchSize:   r.BatchSize,
		RepeatValue: r.RepeatValue,
		RepeatUnit:  strings.ToLower(strings.TrimSpace(r.RepeatUnit)),
		Days:        r.Days,
		StartTime:   r.StartTime,
		EndTime:     r.EndTime,
		Timezone:    strings.TrimSpace(r.Timezone),
	}
}
