App.HostsNewController = Em.ObjectController.extend
  canSelectPlan: true
  name: null
  slug: null
  selectedRegionSlug: null
  selectedPlan: null
  error: null
  invalidField: null
  host: null
  saving: false

  clear: ->
    @setProperties
      name: null
      slug: null
      selectedRegionSlug: null
      selectedPlan: App.get('plans.firstObject')
      error: null
      invalidField: null
      host: null
      saving: false

  canCancel: (->
    !App.get('noHost')
  ).property 'App.noHost'

  plansDidChange: (->
    unless @get('selectedPlan')
      @set('selectedPlan', App.get('plans.firstObject'))
  ).observes 'App.plans.@each'

  actions:
    selectPlan: (plan)->
      @set 'selectedPlan', plan

    cancel: ->
      @transitionToRoute 'home.index'
      false

    create: ->
      host = @get 'host'
      if host
        # restore
        @set 'name', host.get('name')
        @set 'slug', host.get('slug')
      else
        host = App.Host.create
          regionSlug: @get('selectedRegionSlug')
          planSlug: @get('selectedPlan.slug')
          templateSlug: ""
          name: @get 'name'
          slug: @get 'slug'

        @set 'host', host

      if App.get('newUser') && App.get('user.billingEnabled')
        App.set('pendingHost', host)
        @transitionToRoute 'add-card'
        return

      events = App.get 'eventsSocket'
      @set 'saving', true

      onHostCreateFailed = (data) =>
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.create_failed', onHostCreateFailed
        events.off 'host.created', onHostCreated
        App.postError(data.error, 'Failed to create workspace!')
        @set 'saving', false

      onHostCreated = (data) =>
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.create_failed', onHostCreateFailed
        events.off 'host.created', onHostCreated
        @transitionToRoute 'host.index', host.get('slug')
        App.snackbar.post("Your new workspace “#{host.get('name')}” was created successfully.")
        @set 'saving', false

      events.on 'host.create_failed', onHostCreateFailed
      events.on 'host.created', onHostCreated

      params =
        async: true

      host.save(params, json: true).catch (jqxhr) => # host.save
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]
        App.postError(jqxhr.responseJSON, 'Failed to create workspace!')
        @set 'saving', false
