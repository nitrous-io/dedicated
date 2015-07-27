App.slug =
  toSlug: (text, appendNumber) ->
    if text
      slug = text.dasherize().replace(/[^A-Za-z0-9\-]/g, '-')
      if appendNumber
        slug += '-' + util.randomInteger(1000, 9999).toString()
      slug.replace(/-{2,}/g, '-').replace(/^-/, '').replace(/-$/, '')
    else
      ''

  slugify: ($nameField, $slugField, appendNumber=true) ->
    $nameField.on 'input.slugify', (ev) =>
      $slugField.val(@toSlug($.trim(ev.target.value), appendNumber)).change()

  unslugify: ($nameField) ->
    $nameField.off 'input.slugify'

  inputPattern: '[a-z0-9][a-z0-9\-]*'
  inputTitle: 'May only contain alphanumeric characters or dashes and cannot begin with a dash.'
