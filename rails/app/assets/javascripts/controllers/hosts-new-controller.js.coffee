App.HostsNewController = Em.ObjectController.extend
  name: null
  slug: null
  selectedRegionSlug: null
  selectedPlanSlug: null
  error: null
  invalidField: null
  saving: false

  clear: ->
    @setProperties
      name: null
      slug: null
      selectedRegionSlug: null
      selectedPlanSlug: null
      error: null
      invalidField: null
      saving: false

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

      onHostCreateFailed = (data) =>
        return unless data.id == host.get('id')
        events.off 'host.create_failed', onHostCreateFailed
        events.off 'host.created', onHostCreated
        App.snackbar.post('Failed to create host!')
        @set 'saving', false

      onHostCreated = (data) =>
        return unless data.id == host.get('id')
        events.off 'host.create_failed', onHostCreateFailed
        events.off 'host.created', onHostCreated
        @transitionToRoute 'host.index', slug
        App.snackbar.post("Your host “#{name}” was created successfully.")
        @set 'saving', false

      events.on 'host.create_failed', onHostCreateFailed
      events.on 'host.created', onHostCreated

      host.save(async: true).catch (jqxhr) => # host.save
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]
          App.snackbar.post(errMsg?.capitalize() || 'Failed to create host!')
        @set 'saving', false
