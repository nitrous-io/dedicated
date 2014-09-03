App.ContainersNewController = Em.Controller.extend
  selectedHostSlug: null
  name: null
  slug: null
  invalidField: null
  saving: false

  clear: ->
    @setProperties
      selectedHostSlug: null
      name: null
      slug: null
      invalidField: null
      saving: false

  selectedHost: (->
    if slug = @get 'selectedHostSlug'
      App.get('hosts').findBy 'slug', slug
  ).property 'selectedHostSlug', 'App.hosts.@each'

  actions:
    cancel: ->
      @transitionToRoute 'home.index'

    create: (image) ->
      @set 'saving', true
      container = App.Container.create
        name: @get('name')
        slug: @get('slug')
        image: image
      container.save(host: @get 'selectedHostSlug').then =>
        App.get('containers').reload().then =>
          @transitionToRoute 'container.index', container.get 'slug'
          @set 'saving', false
      , (jqxhr) =>
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]
          App.snackbar.post(errMsg.capitalize())
        @set 'saving', false
