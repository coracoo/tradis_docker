import DOMPurify from 'dompurify'
import { marked } from 'marked'

const HTML_ESCAPE_MAP = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;'
}

const SANITIZE_OPTIONS = {
  USE_PROFILES: { html: true },
  FORBID_TAGS: [
    'button',
    'embed',
    'form',
    'iframe',
    'math',
    'object',
    'select',
    'style',
    'svg',
    'textarea'
  ],
  FORBID_ATTR: ['style']
}

const MARKED_OPTIONS = {
  async: false,
  breaks: true,
  gfm: true
}

export function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>"']/g, char => HTML_ESCAPE_MAP[char])
}

function enhanceSafeMarkup(markup) {
  const template = document.createElement('template')
  template.innerHTML = DOMPurify.sanitize(markup, SANITIZE_OPTIONS)

  template.content.querySelectorAll('a[href]').forEach((link) => {
    const href = link.getAttribute('href') || ''
    link.removeAttribute('target')
    link.removeAttribute('rel')
    if (/^(?:https?:)?\/\//i.test(href)) {
      link.setAttribute('target', '_blank')
      link.setAttribute('rel', 'noopener noreferrer')
    }
  })

  template.content.querySelectorAll('input').forEach((input) => {
    const isTaskCheckbox = input.getAttribute('type') === 'checkbox' && input.hasAttribute('disabled')
    if (!isTaskCheckbox) {
      input.remove()
      return
    }

    const isChecked = input.hasAttribute('checked')
    Array.from(input.attributes).forEach(attribute => input.removeAttribute(attribute.name))
    input.setAttribute('type', 'checkbox')
    input.setAttribute('disabled', '')
    if (isChecked) input.setAttribute('checked', '')
  })

  template.content.querySelectorAll('img').forEach((image) => {
    image.setAttribute('loading', 'lazy')
    image.setAttribute('decoding', 'async')
  })

  return template.innerHTML
}

function render(value, inline = false) {
  const source = String(value ?? '').trim()
  if (!source) return ''

  const markup = inline
    ? marked.parseInline(source, MARKED_OPTIONS)
    : marked.parse(source, MARKED_OPTIONS)

  return enhanceSafeMarkup(markup)
}

export function renderSafeMarkdown(value) {
  return render(value)
}

export function renderSafeMarkdownInline(value) {
  return render(value, true)
}
