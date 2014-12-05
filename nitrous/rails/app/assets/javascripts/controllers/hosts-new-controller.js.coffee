App.HostsNewController = Em.ObjectController.extend
  name: null
  slug: null
  selectedRegionSlug: null
  selectedPlanSlug: null
  error: null
  invalidField: null
  host: null
  saving: false

  clear: ->
    @setProperties
      name: null
      slug: null
      selectedRegionSlug: null
      selectedPlanSlug: null
      error: null
      invalidField: null
      host: null
      saving: false

  templates: [
    {
      slug: 'rails'
      iconUrl: 'https://nitrous-image-icons.s3.amazonaws.com/rails.png'
      name: 'Ruby on Rails'
      description: 'A baseline Ruby on Rails application that uses PostgreSQL as the database backend.\n\nThis template contains:\n  •  Ubuntu 14.04\n  •  Ruby 2.1\n  •  Rails 4.1\n  •  PostgreSQL 9.3'
    }
    {
      slug: 'vanilla'
      iconUrl: 'https://nitrous-image-icons.s3.amazonaws.com/vanilla.png'
      name: 'Vanilla'
      description: 'A vanilla development environment.\n\nThis template contains Ubuntu 14.04 with useful developer tools preinstalled.'
    }
  ]

  selectedPlan: (->
    if slug = @get('selectedPlanSlug')
      App.get('plans').findBy('slug', slug)
  ).property 'selectedPlanSlug', 'App.plans.@each'

  actions:
    cancel: ->
      @transitionToRoute 'home.index'
      false

    create: (template) ->
      @set 'saving', true
      name = @get 'name'
      slug = @get 'slug'

      events = App.get 'eventsSocket'

      host = App.Host.create
        regionSlug: @get('selectedRegionSlug')
        planSlug: @get('selectedPlanSlug')
        name: name
        slug: slug
      @set 'host', host

      onHostCreateFailed = (data) =>
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.create_failed', onHostCreateFailed
        events.off 'host.created', onHostCreated
        App.postError(data.error, 'Failed to create application!')
        @set 'saving', false

      onHostCreated = (data) =>
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.create_failed', onHostCreateFailed
        events.off 'host.created', onHostCreated
        @transitionToRoute 'host.index', slug
        App.snackbar.post("Your application “#{name}” was created successfully.")
        @set 'saving', false

      events.on 'host.create_failed', onHostCreateFailed
      events.on 'host.created', onHostCreated

      params =
        async: true

      switch template
        when 'rails'
          workspaceVolSlug = 'workspace'
          databaseVolSlug = 'database'

          params.volumes = [
            { name: 'workspace', slug: workspaceVolSlug }
            { name: 'database',  slug: databaseVolSlug }
          ]

          params.containers = [
            { name: 'postgres', slug: App.slug.toSlug('postgres', true), image: 'postgres:9.3', volumes: ["#{databaseVolSlug}:/var/lib/postgresql/data"], ports: ["private:5432:5432"] }
            { name: 'app', slug: App.slug.toSlug('app', true), image: 'nitrousio/rails-pg-jumpstart:latest', volumes: ["#{workspaceVolSlug}:/volumes/workspace"], ports: ["public:3000:3000", "public:2222:22"] }
          ]
        when 'vanilla'
          params.containers = [
            { name: 'app', slug: App.slug.toSlug('app', true), image: 'nitrousio/ubuntu:trusty', volumes: [], ports: ["public:2222:22"] }
          ]

      host.save(params, json: true).catch (jqxhr) => # host.save
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]
        App.postError(jqxhr.responseJSON, 'Failed to create application!')
        @set 'saving', false
