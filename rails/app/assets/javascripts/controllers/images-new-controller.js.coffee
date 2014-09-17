App.ImagesNewController = Em.Controller.extend
  queryParams: ['preselectHost']
  preselectHost: null

  selectedHostSlug: null
  name: null
  slug: null
  image: null
  saving: false

  clear: ->
    @setProperties
      selectedHostSlug: null
      name: null
      slug: null
      image: null
      saving: false

  actions:
    cancel: ->
      if slug = @get('preselectHost')
        @transitionToRoute 'host.images', slug
      else
        @transitionToRoute 'home.index'

    create: (file) ->
      @set 'saving', true
      name = @get 'name'
      slug = @get 'slug'
      hostSlug = @get 'selectedHostSlug'

      data = new FormData()
      data.append 'name', name
      data.append 'slug', slug
      data.append 'async', true
      data.append 'file', file

      events = App.get 'eventsSocket'

      image = App.Image.create(name: name, slug: slug)
      @set 'image', image

      events.on 'image.build_failed', onImageBuildFailed = (data) =>
        return unless data.id == image.get('id')
        events.off 'image.build_failed', onImageBuildFailed
        @set 'image', null
        if data.progress
          progress = JSON.parse(progress)
          errMsg = progress.errorDetail?.message
          App.snackbar.post(errMsg || 'Failed to build image!')
        @set 'saving', false

      events.on 'image.built', onImageBuilt = (data) =>
        return unless data.id == image.get('id')
        events.off 'image.built', onImageBuilt

        @transitionToRoute 'host.images', hostSlug
        App.snackbar.post("Your image “#{name}” was built successfully.")
        @set 'saving', false

      Em.RSVP.Promise.cast(Em.$.ajax(
        type: 'POST'
        url: "/api/v0/hosts/#{hostSlug}/images"
        data: data
        cache: false
        contentType: false
        processData: false
        headers: App.Image.adapter.get('headers')
      )).then (json) =>
        image.load(json.image)
      , (jqxhr) -> # Em.RSVP.Promise.cast
        errMsg = jqxhr.responseJSON?.error_description
        App.snackbar.post(errMsg?.capitalize() || 'Failed to build image!')
        @set 'saving', false
