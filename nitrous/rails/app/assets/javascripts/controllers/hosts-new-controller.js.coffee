App.HostsNewController = Em.ObjectController.extend App.PromoCodeInfoMixin,
  queryParams: ['pendingContainer']
  pendingContainer: false

  canSelectPlan: true
  name: null
  selectedRegion: null
  selectedPlan: null
  error: null
  invalidField: null
  host: null
  saving: false
  bypassPreprovision: false
  templateName: null
  promoCodeBinding: Em.Binding.oneWay 'App.user.promoCode'

  nameFieldEdited: false

  clear: ->
    @setProperties
      name: null
      selectedRegion: App.get('regions.firstObject')
      selectedPlan: App.get('plans.firstObject')
      error: null
      invalidField: null
      host: null
      saving: false
      bypassPreprovision: false
      templateName: null
      nameFieldEdited: false
    if @get('hasPendingContainer')
      @set('name', App.get('user.username'))

  hasPendingContainer: (->
    @get('pendingContainer') && App.get('pendingContainerData')
  ).property 'pendingContainer', 'App.pendingContainerData'

  plansDidChange: (->
    unless @get('selectedPlan')
      @set('selectedPlan', App.get('plans.firstObject'))
  ).observes 'App.plans.@each'

  regionsDidChange: (->
    unless @get('selectedRegion')
      @set('selectedRegion', App.get('regions.firstObject'))
  ).observes 'App.regions.@each'

  actions:
    selectPlan: (plan)->
      @set 'selectedPlan', plan

    cancel: ->
      @transitionToRoute 'home.index'
      false

    goBack: ->
      if App.get('pendingContainerData')
        App.set('pendingContainerData', null)
        @transitionToRoute 'containers.new'
        return
      @transitionToRoute 'home.index'

    create: (host) ->
      if !host
        host = App.Host.create
          regionSlug: @get('selectedRegion.slug')
          regionId: @get('selectedRegion.id')
          planSlug: @get('selectedPlan.slug')
          planId: @get('selectedPlan.id')
          name: @get 'name'
      @set 'host', host

      if App.get('newUser')
        App.set('pendingHost', host)
        @transitionToRoute 'add-card'
        return

      if pendingContainerData = App.get('pendingContainerData')
        if App.get('onboardingScreen')
          App.set 'onboardingScreen', 'thank-you'

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

        # Check if there's a pending container that we should now create.
        if pendingContainerData = App.get('pendingContainerData')
          pendingContainerData.set('selectedHostSlug', host.get('slug'))
          @transitionToRoute 'containers.new', queryParams: { runCreate: true }
        else
          @transitionToRoute 'host.index', host.get('slug')
          App.snackbar.post("Your new workspace “#{host.get('slug')}” was created successfully.", sticky: true)

        @set 'saving', false

      events.on 'host.create_failed', onHostCreateFailed
      events.on 'host.created', onHostCreated

      params =
        async: true
        bypass_preprovision: @get('bypassPreprovision')
        template_name: @get('templateName')

      host.save(params, json: true).catch (jqxhr) => # host.save
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]
        App.postError(jqxhr.responseJSON, 'Failed to create workspace!')
        @set 'saving', false
