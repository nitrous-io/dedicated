App.ImagesNewController = Em.Controller.extend
  queryParams: ['preselectHost']
  preselectHost: null

  selectedHostSlug: null
  name: null
  slug: null
  saving: false

  clear: ->
    @setProperties
      selectedHostSlug: null
      name: null
      slug: null
      saving: false

  actions:
    cancel: ->
      if slug = @get('preselectHost')
        @transitionToRoute 'host.images', slug
      else
        @transitionToRoute 'home.index'

    create: (file) ->
      @set 'saving', true
      name = @get 'slug'
      slug = @get 'slug'
      data = new FormData()
      data.append 'name', name
      data.append 'slug', slug
      data.append 'file', file

      image = App.Image.create(name: name, slug: slug)
      Em.RSVP.Promise.cast(Em.$.ajax(
        type: 'POST'
        url: "/api/v0/hosts/#{@get 'selectedHostSlug'}/images"
        data: data
        cache: false
        contentType: false
        processData: false
        headers: App.Image.adapter.get('headers')
      )).then((json) =>
        image.load(json.image)
        @transitionToRoute 'home.index'
        App.snackbar.post("Your image “#{name}” was successfully built.")
      , (jqxhr) ->
        errMsg = jqxhr.responseJSON?.error_description
        App.snackbar.post(errMsg?.capitalize() || 'Unknown Error')
      ).finally =>
        @set 'saving', false
