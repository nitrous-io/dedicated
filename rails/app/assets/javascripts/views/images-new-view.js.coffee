Blob = window.Blob || window.WebKitBlob

App.ImagesNewView = Em.View.extend
  elementId: 'images-new-view'
  classNames: ['ui', 'card']

  codeMirror: null

  selectedOption: null
  selectedFile: null
  selectedFileName: null
  dockerfileCode: null

  createButtonDisabled: (->
    !(
      @get('controller.name') && @get('controller.slug') && @get('controller.selectedHostSlug') && (
        (@get('selectedOption') == 'file' && @get('selectedFile')) ||
        (@get('selectedOption') == 'code' && @get('dockerfileCode'))
      )
    )
  ).property 'controller.name', 'controller.slug', 'controller.selectedHostSlug', 'selectedOption', 'selectedFile', 'dockerfileCode'

  hostsDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      slug = @get('controller.hostQueryParam')
      if slug && !@get('selectedHostSlug')
        @set 'controller.selectedHostSlug', slug
      @$('select[name=host]')?.change()
  ).observes 'App.hosts.@each'

  hostQueryParamDidChange: (->
    if slug = @get('controller.hostQueryParam')
      @set 'controller.selectedHostSlug', slug
  ).observes 'controller.hostQueryParam'

  didInsertElement: ->
    @_super()
    @set 'codeMirror', codemirror = CodeMirror.fromTextArea @$('#dockerfile-code')[0],
      lineNumbers: true
      viewportMargin: Infinity

    codemirror.changeHandler = =>
      @set 'dockerfileCode', codemirror.getValue()
    codemirror.on 'change', codemirror.changeHandler

    @$('.editor .CodeMirror').on 'click.images-new', (ev) =>
      Em.run.next this, ->
        @$('#images-new-option-code').click()

    @$('#images-new-file').on 'click.images-new', (ev) =>
      Em.run.next this, ->
        @$('#images-new-option-file').click()

    @$('input[name=option]').on('change.images-new', (ev) =>
      if $(ev.target).is(':checked')
        @set 'selectedOption', ev.target.value
    ).change()

    App.slug.slugify(@$('#images-new-name'), @$('#images-new-slug'), false)

    @hostsDidChange()
    @$('#images-new-name').focus()

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#images-new-name'))
    @$('input[name=option]').off('change.images-new')
    @$('#images-new-file').off('click.images-new')
    @$('.editor .CodeMirror').off('click.images-new')
    if codeMirror = @get('codeMirror')
      codeMirror.off 'change', codeMirror.changeHandler
      codeMirror.changeHandler = null
      codeMirror.toTextArea()
    @set 'codeMirror', null

  actions:
    didChangeFile: ->
      if files = @$('#images-new-file')[0].files
        file = files[0]
        @set 'selectedFile', file
        @set 'selectedFileName', file?.name

    create: ->
      file = switch @get('selectedOption')
        when 'file'
          @get 'selectedFile'
        when 'code'
          new Blob([@get 'dockerfileCode'], type: 'text/plain')
      @get('controller').send 'create', file
