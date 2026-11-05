package main

import (
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/compdani/list_pocket/internal/apperr"
)

func deliveryReceipt(rec *pbcore.Record) map[string]any {
	status := rec.GetString("status")
	return map[string]any{"delivery_id": rec.Id, "status": status, "queued": status == "queued" || status == "sending", "sent": status == "sent"}
}
func (a *App) getFlowDelivery(re *pbcore.RequestEvent) error {
	rec, err := a.pb.FindRecordById("flow_deliveries", pathParam(re, "id"))
	if err != nil {
		return apperr.NotFound("delivery not found")
	}
	receipt := deliveryReceipt(rec)
	receipt["error"] = rec.GetString("error")
	return okJSON(re, receipt)
}
