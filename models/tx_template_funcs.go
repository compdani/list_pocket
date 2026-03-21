package models

import (
	"html/template"
	"maps"
	txttpl "text/template"
)

func txAliasData(sub Subscriber, msg *TxMessage) (map[string]any, map[string]any) {
	contactData := map[string]any{
		"id":        sub.RecordID,
		"email":     sub.Email,
		"name":      sub.Name,
		"firstName": sub.FirstName,
		"lastName":  sub.LastName,
		"phone":     sub.Phone,
		"attribs":   sub.Attribs,
	}

	txData := map[string]any{}
	if msg != nil {
		txData = map[string]any{
			"data":        msg.Data,
			"fromEmail":   msg.FromEmail,
			"contentType": msg.ContentType,
			"messenger":   msg.Messenger,
			"subject":     msg.Subject,
		}
	}

	return contactData, txData
}

func TxAliasTemplateFuncs(base template.FuncMap, sub Subscriber, msg *TxMessage) template.FuncMap {
	out := template.FuncMap{}
	maps.Copy(out, base)

	contactData, txData := txAliasData(sub, msg)
	out["contact"] = func() map[string]any { return contactData }
	out["subscriber"] = func() map[string]any { return contactData }
	out["tx"] = func() map[string]any { return txData }
	out["data"] = func() map[string]any {
		if msg == nil || msg.Data == nil {
			return map[string]any{}
		}
		return msg.Data
	}

	return out
}

func TxAliasTextFuncs(base txttpl.FuncMap, sub Subscriber, msg *TxMessage) txttpl.FuncMap {
	out := txttpl.FuncMap{}
	maps.Copy(out, base)

	contactData, txData := txAliasData(sub, msg)
	out["contact"] = func() map[string]any { return contactData }
	out["subscriber"] = func() map[string]any { return contactData }
	out["tx"] = func() map[string]any { return txData }
	out["data"] = func() map[string]any {
		if msg == nil || msg.Data == nil {
			return map[string]any{}
		}
		return msg.Data
	}

	return out
}
