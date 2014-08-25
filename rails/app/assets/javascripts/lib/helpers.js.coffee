window.helpers =
  slugify: (text) ->
    slug = text.dasherize().replace(/[^A-Za-z0-9\-]/g, '-')
    if slug.charAt(0) == '-'
      return 'a-' + slug
    slug

  escapeRegex: (text) ->
    text.replace(/([.*+?^${}()|\[\]\/\\])/g, "\\$1")
