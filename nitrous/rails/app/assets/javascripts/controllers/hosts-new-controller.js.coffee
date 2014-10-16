App.HostsNewController = Em.ObjectController.extend
  name: null
  slug: null
  selectedRegionSlug: null
  selectedPlanSlug: null
  error: null
  invalidField: null
  host: null
  saving: false
  useJumpstartImage: false

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
      useJumpstartImage: false

  selectedPlan: (->
    if slug = @get('selectedPlanSlug')
      App.get('plans').findBy('slug', slug)
  ).property 'selectedPlanSlug', 'App.plans.@each'

  actions:
    cancel: ->
      @transitionToRoute 'home.index'
      false

    create: ->
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
        App.postError(data.error, 'Failed to create host!')
        @set 'saving', false

      onHostCreated = (data) =>
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.create_failed', onHostCreateFailed
        events.off 'host.created', onHostCreated
        @transitionToRoute 'host.index', slug
        App.snackbar.post("Your host “#{name}” was created successfully.")
        @set 'saving', false

      events.on 'host.create_failed', onHostCreateFailed
      events.on 'host.created', onHostCreated

      params =
        async: true

      if @get 'useJumpstartImage'
        workspaceVolSlug = App.slug.toSlug('workspace', true)
        databaseVolSlug = App.slug.toSlug('database', true)

        params.volumes = [
          { name: 'workspace', slug: workspaceVolSlug }
          { name: 'database',  slug: databaseVolSlug }
        ]

        params.containers = [
          { name: 'postgres', slug: App.slug.toSlug('postgres', true), image: 'postgres:9.3', volumes: ["#{databaseVolSlug}:/var/lib/postgresql/data"], ports: ["5432:5432"] }
          { name: 'rails', slug: App.slug.toSlug('rails', true), image: 'nitrousio/rails-pg-jumpstart:latest', volumes: ["#{workspaceVolSlug}:/volumes/workspace"], ports: ["3000:3000", "2222:22"] }
        ]

      host.save(params, json: true).catch (jqxhr) => # host.save
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]
        App.postError(jqxhr.responseJSON, 'Failed to create host!')
        @set 'saving', false
