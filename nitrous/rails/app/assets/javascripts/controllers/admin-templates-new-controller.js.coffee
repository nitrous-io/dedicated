defaultConfig = '{
  "ports": [],
  "publish_all": true,
  "env": [],
  "volumes": [],
  "privileged": false
}'

App.AdminTemplatesNewController = Em.Controller.extend
  templateTypes: ["basic", "stack", "advanced"]

  slug: null
  name: null
  description: null
  image: null
  type: null
  config: defaultConfig

  saving: false

  clear: ->
    @setProperties
      slug: null
      name: null
      description: null
      image: null
      type: null
      config: defaultConfig
      saving: false

  actions:
    cancel: ->
      @transitionToRoute 'admin.templates'
      false

    create: ->
      @set 'saving', true
      payload =
        slug: @get('slug')
        name: @get('name')
        description: @get('description')
        image: @get('image')
        type: @get('type')
        config: @get('config')

      App.Model.adapter.ajax('POST', '/admin/templates', payload).done( =>
        App.snackbar.post("Successfully created template #{@get 'slug'}")
        @set 'saving', false
        @transitionToRoute 'admin.templates'
      ).fail (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to create template")
        @set 'saving', false
      false
