// Syntax theme matching the "Cartographe" palette: ink background, parchment
// text, gold for keywords, muted sage and clay for literals.
export default {
  name: 'cartographe',
  type: 'dark',
  colors: {
    'editor.background': '#13232A',
    'editor.foreground': '#ECE2CB',
  },
  tokenColors: [
    { scope: ['comment', 'punctuation.definition.comment'], settings: { foreground: '#8A9E9C', fontStyle: 'italic' } },
    { scope: ['keyword', 'storage', 'storage.type', 'keyword.control', 'keyword.operator.new'], settings: { foreground: '#E9B44C' } },
    { scope: ['string', 'string.quoted', 'markup.inline.raw'], settings: { foreground: '#B9CF9A' } },
    { scope: ['constant.numeric', 'constant.language', 'constant.character', 'support.constant'], settings: { foreground: '#E79A73' } },
    { scope: ['entity.name.function', 'support.function', 'meta.function-call'], settings: { foreground: '#9CC9D3' } },
    { scope: ['entity.name.type', 'support.type', 'entity.name.class', 'entity.other.inherited-class'], settings: { foreground: '#D9C48E' } },
    { scope: ['variable.parameter', 'variable.other.readwrite', 'variable'], settings: { foreground: '#ECE2CB' } },
    { scope: ['entity.name.tag', 'support.type.property-name', 'entity.other.attribute-name', 'meta.object-literal.key'], settings: { foreground: '#9CC9D3' } },
    { scope: ['punctuation', 'meta.brace', 'keyword.operator'], settings: { foreground: '#B7AE98' } },
    { scope: ['variable.other.constant', 'variable.language'], settings: { foreground: '#E79A73' } },
    { scope: ['markup.heading', 'entity.name.section'], settings: { foreground: '#E9B44C', fontStyle: 'bold' } },
    { scope: ['markup.inserted'], settings: { foreground: '#B9CF9A' } },
    { scope: ['markup.deleted'], settings: { foreground: '#E79A73' } },
    { scope: ['support.function.builtin.shell', 'entity.name.command'], settings: { foreground: '#9CC9D3' } },
  ],
};
