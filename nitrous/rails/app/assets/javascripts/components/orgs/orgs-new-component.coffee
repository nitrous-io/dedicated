App.OrgsNewComponent = Em.Component.extend
  layoutName: 'components/orgs/new'
  elementId: 'orgs-new'
  classNames: ['ui', 'card']

  model: null
  saving: false

  createButtonDisabled: (->
    !(@get('model.name') && @get('model.slug') && @get('model.email'))
  ).property 'model.name', 'model.slug', 'model.email'

  didInsertElement: ->
    @_super()
    App.slug.slugify(@$('#orgs-new-name'), @$('#orgs-new-slug'), false)
    Em.run.next =>
      @focus()

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#orgs-new-name'))

  focus: ->
    @$('#orgs-new-name').focus().select()

  actions:
    create: ->
      @set 'saving', true
      model = @get('model')
      model.save().then(=>
        App.snackbar.post("Your organization “#{model.get 'name'}” was successfully created.")
        PubSub.publish 'orgs-new.created', model
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Could not create organization, please try again.')
        @focus()
      ).finally =>
        @set 'saving', false
