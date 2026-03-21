package main

import (
	"github.com/compdani/list_pocket/internal/txemail"
	"github.com/compdani/list_pocket/internal/workflow"
	"github.com/compdani/list_pocket/models"
)

func (a *App) newTransactionalSender() *txemail.Sender {
	return &txemail.Sender{
		Core:             a.core,
		Manager:          a.manager,
		DefaultFromEmail: a.cfg.FromEmail,
		Log:              a.log,
	}
}

func txRequestFromWorkflow(req workflow.ExecutorTransactionalEmailRequest) txemail.Request {
	return txemail.Request{
		TemplateID:      req.TemplateID,
		SubscriberID:    req.SubscriberID,
		SubscriberEmail: req.SubscriberEmail,
		SubscriberName:  req.SubscriberName,
		FirstName:       req.FirstName,
		LastName:        req.LastName,
		Phone:           req.Phone,
		Attribs:         models.JSON(req.Attribs),
		Data:            req.Data,
		FromEmail:       req.FromEmail,
		Subject:         req.Subject,
		ContentType:     req.ContentType,
		Messenger:       req.Messenger,
	}
}
