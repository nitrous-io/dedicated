App.HostsNewController = Em.ObjectController.extend App.PromoCodeInfoMixin,
  canSelectPlan: true
  name: null
  selectedRegion: null
  selectedPlan: null
  error: null
  invalidField: null
  host: null
  saving: false
  promoCodeBinding: Em.Binding.oneWay 'App.user.promoCode'

  clear: ->
    @setProperties
      name: null
      selectedRegion: App.get('regions.firstObject')
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

  regionsDidChange: (->
    unless @get('selectedRegion')
      @set('selectedRegion', App.get('regions.firstObject'))
  ).observes 'App.regions.@each'

  promoCreditNotYetReceived: (->
    App.get('billingProfile.cardRequired') && App.get('billingUsage.totalCharge') == 0
  ).property 'App.billingProfile.cardRequired', 'App.billingUsage.totalCharge'

  availableCredit: (->
     if @get 'promoCreditNotYetReceived'
       @get 'promoCodeInfo.content'
     else
       return App.get 'billingUsage.availableCreditsDollars'
  ).property 'promoCreditNotYetReceived', 'promoCodeInfo.content', 'App.billingUsage.availableCreditsDollars'

  actions:
    selectPlan: (plan)->
      @set 'selectedPlan', plan

    cancel: ->
      @transitionToRoute 'home.index'
      false

    create: (host) ->
      if !host
        host = App.Host.create
          regionSlug: @get('selectedRegion.slug')
          planSlug: @get('selectedPlan.slug')
          name: @get 'name'
      @set 'host', host

      if App.get('newUser')
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
        App.snackbar.post("Your new workspace “#{host.get('slug')}” was created successfully.")
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
