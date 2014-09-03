App.ContainersNewView = Em.View.extend
  elementId: 'containers-new-view'
  classNames: ['ui', 'card']

  showAdvanced: false

  selectedOption: null
  selectedHostImage: null
  selectedBaseImage: null
  customImage: null

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')
  slugFieldInvalid: Em.computed.equal('controller.invalidField', 'slug')

  image: (->
    option = @get 'selectedOption'
    switch option
      when 'host-image'
        @get 'selectedHostImage'
      when 'base-image'
        @get 'selectedBaseImage'
      else
        @get 'customImage'
  ).property 'selectedOption', 'selectedHostImage', 'selectedBaseImage', 'customImage'

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug') && @get('controller.selectedHostSlug') && @get('image'))
  ).property 'controller.name', 'controller.slug', 'controller.selectedHostSlug', 'image'

  hostsDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('select[name=host]')?.change()
  ).observes 'App.hosts.@each'

  hostImagesDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('select[name=containers-new-option-host-image]')?.change()
  ).observes 'controller.selectedHost.images.@each'

  didInsertElement: ->
    @_super()
    App.slug.slugify(@$('#containers-new-name'), @$('#containers-new-slug'))

    @$('input[name=option]').on('change.containers-new', (ev) =>
      if $(ev.target).is(':checked')
        @set 'selectedOption', ev.target.value
    ).change()

    @$('.option-input').on 'click.containers-new', (ev) =>
      Em.run.next this, ->
        @$("##{$(ev.target).attr('name')}").click()

    @$('#containers-new-name').focus()

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#containers-new-name'))
    @$('.option-input').off('click.containers-new')
    @$('input[name=option]').off('change.containers-new')

  actions:
    toggleShowAdvanced: ->
      @$('#toggle-advanced-btn').blur()
      @toggleProperty 'showAdvanced'
      false

    create: ->
      @get('controller').send 'create', @get('image')
