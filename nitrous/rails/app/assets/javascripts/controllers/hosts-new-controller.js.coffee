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
  savingBinding: 'App.hostSaving'
  bypassPreprovision: false
  templateName: null
  plansBinding: Em.Binding.oneWay 'App.eligiblePlans'
  promoCodeBinding: Em.Binding.oneWay 'App.user.promoCode'
  sortedPlans: null
  countryDict: null
  freePlanNotAllowed: false

  nameFieldEdited: false

  paidPlans: (->
    @get('sortedPlans').filterBy('isPaidPlan')
  ).property 'sortedPlans.@each'

  freePlans: (->
    @get('sortedPlans').filterBy('isFreePlan')
  ).property 'sortedPlans.@each'

  clear: ->
    plans = @get 'plans'

    sortedPlans = Em.ArrayProxy.createWithMixins Em.SortableMixin,
      contentBinding: Em.Binding.oneWay 'App.eligiblePlans'
      sortProperties: ['totalPrice']

    @setProperties
      name: null
      sortedPlans: sortedPlans
      selectedPlan: plans?.findBy('isPaidPlan')
      selectedRegion: App.get('regions.firstObject')
      error: null
      invalidField: null
      host: null
      saving: false
      bypassPreprovision: false
      templateName: null
      nameFieldEdited: false
      countryDict: null
      freePlanNotAllowed: false

    if @get('hasPendingContainer')
      @set('name', App.get('user.username'))

    plans?.reload()

  hasPendingContainer: (->
    @get('pendingContainer') && App.get('pendingContainerData')
  ).property 'pendingContainer', 'App.pendingContainerData'

  plansDidChange: (->
    @set('selectedPlan', @get('plans')?.findBy('isPaidPlan'))
  ).observes 'plans.@each'

  regionsDidChange: (->
    unless @get('selectedRegion')
      @set('selectedRegion', App.get('regions.firstObject'))
  ).observes 'App.regions.@each'

  cardRequired: (->
    if App.get('billingProfile.exempt')
      return false

    # otherwise a paid plan must have a card
    if @get('selectedPlan.isPaidPlan')
      return !App.get('billingProfile.hasCard')

    false
  ).property 'App.billingProfile.hasCard', 'selectedPlan.isPaidPlan'

  hasNextStep: (->
    @get('cardRequired')
  ).property 'cardRequired'

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
      else
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

      if @get('selectedPlan.isFreePlan') && !App.get('user.phoneAuthed')
        App.set('pendingHost', host)

        @transitionTo 'add-phone'
        return

      if @get('cardRequired')
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
        container_template_slug: App.get('pendingContainerData.selectedTemplate.slug')
        container_name: App.get('pendingContainerData.name')

      host.save(params, json: true).then(=>
        if pendingContainerData = App.get('pendingContainerData')
          if App.get('onboardingScreen')
            App.set 'onboardingScreen', 'thank-you'
      ).catch (jqxhr) =>
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]

        if jqxhr.responseJSON?.error == "free_plan_not_allowed"
          @set 'freePlanNotAllowed', true
        App.postError(jqxhr.responseJSON, 'Failed to create workspace!')
        @set 'saving', false
