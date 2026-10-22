import MarkdownIt from 'markdown-it';

const markdown = new MarkdownIt({ html: false, breaks: true });
// Replies may contain links, but should not load remote tracking images.
markdown.disable('image');
const linkOpen = markdown.renderer.rules.link_open
  || ((tokens, index, options, env, renderer) => renderer.renderToken(tokens, index, options));
markdown.renderer.rules.link_open = (tokens, index, options, env, renderer) => {
  tokens[index].attrSet('target', '_blank');
  tokens[index].attrSet('rel', 'noopener noreferrer');
  return linkOpen(tokens, index, options, env, renderer);
};
export function renderAssistantMarkdown(text) {
  return markdown.render(String(text || ''));
}
