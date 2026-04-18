export function legacyActivityToTimeline(data) {
  const toDate = (raw) => (raw ? raw : null);

  const campaignSends = Array.isArray(data?.campaignSends) ? data.campaignSends : [];
  const campaignViews = Array.isArray(data?.campaignViews) ? data.campaignViews : [];
  const linkClicks = Array.isArray(data?.linkClicks) ? data.linkClicks : [];

  const events = [];

  campaignSends.forEach((row) => {
    events.push({
      eventType: 'campaign_send',
      channel: 'email',
      occurredAt: toDate(row.updated || row.created),
      source: 'campaign_send_ledger',
      status: row.status || '',
      actor: {
        type: 'campaign',
        id: row.id || '',
        label: row.name || '',
      },
      metadata: {
        campaignId: row.id || '',
        campaignUuid: row.uuid || '',
        campaignName: row.name || '',
        subject: row.subject || '',
      },
    });
  });

  campaignViews.forEach((row) => {
    events.push({
      eventType: 'campaign_view',
      channel: 'email',
      occurredAt: toDate(row.lastViewedAt),
      source: 'campaign_views',
      status: 'viewed',
      actor: {
        type: 'campaign',
        id: row.id || '',
        label: row.name || '',
      },
      metadata: {
        campaignId: row.id || '',
        campaignUuid: row.uuid || '',
        campaignName: row.name || '',
        subject: row.subject || '',
        viewCount: row.viewCount || 0,
      },
    });
  });

  linkClicks.forEach((row) => {
    events.push({
      eventType: 'link_click',
      channel: 'email',
      occurredAt: toDate(row.lastClickedAt),
      source: 'link_clicks',
      status: 'clicked',
      actor: {
        type: 'campaign',
        id: row.campaignId || '',
        label: row.campaignName || '',
      },
      metadata: {
        campaignId: row.campaignId || '',
        campaignUuid: row.campaignUuid || '',
        campaignName: row.campaignName || '',
        subject: row.campaignSubject || '',
        url: row.url || '',
        clickCount: row.clickCount || 0,
      },
    });
  });

  events.sort((a, b) => {
    const ta = a.occurredAt ? new Date(a.occurredAt).getTime() : 0;
    const tb = b.occurredAt ? new Date(b.occurredAt).getTime() : 0;
    return tb - ta;
  });

  return {
    events,
    total: events.length,
    hasMore: false,
  };
}
