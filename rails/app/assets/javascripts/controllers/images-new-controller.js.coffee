App.ImagesNewController = Em.Controller.extend
  selectedHostSlug: null
  name: null

  actions:
    cancel: ->
      @transitionToRoute 'home.index'

    create: (file) ->
      name = @get 'name'
      data = new FormData()
      data.append 'name', name
      data.append 'file', file

      image = App.Image.create(name: name)
      Em.$.ajax(
        type: 'POST'
        url: "/api/v0/hosts/#{@get 'selectedHostSlug'}/images"
        data: data
        cache: false
        contentType: false
        processData: false
        headers: App.Image.adapter.get('headers')
      ).then (json) =>
        image.load(json.image)
        @transitionToRoute 'home.index'
        App.snackbar.post("Your image “#{name}” was successfully built.")
      , (jqxhr) ->
        errMsg = jqxhr.responseJSON?.error_description
        App.snackbar.post(errMsg?.capitalize() || 'Unknown Error')
