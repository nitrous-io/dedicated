App.AdminTemplateEditController = Em.Controller.extend
  templateTypes: ["basic", "stack", "advanced"]

  currentSlug: null

  slug: null
  name: null
  description: null
  image: null
  type: null
  config: null

  saving: false

  loadTemplate: ->
    currentSlug = @get('currentSlug')
    return unless currentSlug?
    App.Model.adapter.ajax('GET', "/admin/template/#{currentSlug}").done((d) =>
      t = d.template
      @setProperties
        slug: t.slug
        name: t.name
        description: t.description
        image: t.image
        type: t.type
        config: t.config
    ).fail (jqxhr) =>
      App.postError(jqxhr.responseJSON, "Failed to load template")
      @transitionToRoute 'admin.templates'

  actions:
    cancel: ->
      @transitionToRoute 'admin.templates'
      false

    update: ->
      @set 'saving', true
      payload =
        slug: @get('slug')
        name: @get('name')
        description: @get('description')
        image: @get('image')
        type: @get('type')
        config: @get('config')

      App.Model.adapter.ajax('PUT', "/admin/template/#{@get 'currentSlug'}", payload).done(=>
        App.snackbar.post("Successfully updated template #{@get 'slug'}")
        @set 'saving', false
        @transitionToRoute 'admin.templates'
      ).fail (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to update template")
        @set 'saving', false
      false
