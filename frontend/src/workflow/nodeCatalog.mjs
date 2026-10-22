const catalog = {
  trigger: ['Start', 'Choose what starts this workflow.', 'mdi-rocket-launch-outline', 'success'],
  condition: ['Logic', 'Split the flow into yes and no branches.', 'mdi-filter-variant', 'warning'],
  transform: ['Logic', 'Transform data with a JavaScript step.', 'mdi-code-braces', 'primary'],
  wait_until: ['Timing', 'Pause before continuing to the next step.', 'mdi-alarm', 'secondary'],
  event_start: ['Timing', 'Schedule steps relative to an event.', 'mdi-calendar-clock', 'secondary'],
  http_request: ['Actions', 'Call an API and use its response.', 'mdi-link-variant', 'primary'],
  pb_update: ['Actions', 'Create or update a contact record.', 'mdi-account-outline', 'primary'],
  send_transactional_email: ['Actions', 'Send a personalized transactional email.', 'mdi-email-outline', 'primary'],
  campaign_launch: ['Actions', 'Launch a campaign for your audience.', 'mdi-send', 'primary'],
};
export function nodePresentation(type) {
  const [category, description, icon, color] = catalog[type] || ['Actions', 'Configure this workflow step.', 'mdi-cog-outline', 'primary'];
  return { category, description, icon, color };
}
