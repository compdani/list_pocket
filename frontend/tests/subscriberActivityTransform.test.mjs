import test from 'node:test';
import assert from 'node:assert/strict';

import { legacyActivityToTimeline } from '../src/components/subscriberActivityTransform.js';

test('legacyActivityToTimeline maps and sorts events desc by occurredAt', () => {
  const out = legacyActivityToTimeline({
    campaignSends: [
      {
        id: 'camp1',
        uuid: 'uuid1',
        name: 'Campaign One',
        subject: 'Subject One',
        status: 'sent',
        created: '2026-04-16T10:00:00Z',
        updated: '2026-04-16T11:00:00Z',
      },
    ],
    campaignViews: [
      {
        id: 'camp1',
        uuid: 'uuid1',
        name: 'Campaign One',
        subject: 'Subject One',
        viewCount: 2,
        lastViewedAt: '2026-04-17T10:00:00Z',
      },
    ],
    linkClicks: [
      {
        campaignId: 'camp1',
        campaignUuid: 'uuid1',
        campaignName: 'Campaign One',
        campaignSubject: 'Subject One',
        url: 'https://example.com',
        clickCount: 3,
        lastClickedAt: '2026-04-17T09:00:00Z',
      },
    ],
  });

  assert.equal(out.total, 3);
  assert.equal(out.hasMore, false);
  assert.equal(out.events[0].eventType, 'campaign_view');
  assert.equal(out.events[1].eventType, 'link_click');
  assert.equal(out.events[2].eventType, 'campaign_send');
});

test('legacyActivityToTimeline handles empty payload', () => {
  const out = legacyActivityToTimeline(null);
  assert.deepEqual(out, {
    events: [],
    total: 0,
    hasMore: false,
  });
});
