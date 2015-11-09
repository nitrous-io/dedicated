App.HostsNewController = Em.ObjectController.extend App.PromoCodeInfoMixin,
  needs: ['orgs']
  queryParams: ['pendingContainer']
  pendingContainer: false

  canSelectPlan: true
  name: null
  selectedRegion: null
  selectedPlan: null
  selectedOrgSlug: null
  error: null
  invalidField: null
  host: null
  savingBinding: 'App.hostSaving'
  templateName: null
  plansBinding: Em.Binding.oneWay 'App.plans'
  promoCodeBinding: Em.Binding.oneWay 'App.user.promoCode'
  sortedPlans: null
  sortedOrgPlans: null
  countryDict: null
  freePlanNotAllowed: false

  nameFieldEdited: false

  orgs: Em.computed.alias 'controllers.orgs.collection'

  orgPlans: null

  selectedOrg: (->
    if slug = @get 'selectedOrgSlug'
      @get('orgs')?.findBy('slug', slug)
  ).property 'orgs.@each', 'selectedOrgSlug'

  paidPlans: (->
    @get('sortedPlans').filter (plan) ->
      plan.get('isPaidPlan')
  ).property 'sortedPlans.@each'

  freePlans: (->
    @get('sortedPlans').filter (plan) ->
      plan.get('isFreePlan')
  ).property 'sortedPlans.@each'

  selectedOrgSlugDidChange: (->
    if @get('selectedOrgSlug')
      App.OrgPlan.fetch(orgSlug: @get('selectedOrgSlug')).then (orgPlans) =>
        @set('orgPlans', orgPlans)

        sortedOrgPlans = Em.ArrayProxy.createWithMixins Em.SortableMixin,
          content: orgPlans
          sortProperties: ['totalPrice']

        @set 'sortedOrgPlans', sortedOrgPlans
        @set 'selectedPlan', @get('sortedOrgPlans.firstObject')
    else
      # if user wants to create a host personally and currently selected plan is an org plan
      @set 'selectedPlan', @get('paidPlans.firstObject')
  ).observes 'selectedOrgSlug'

  clear: ->
    plans = @get 'plans'

    sortedPlans = Em.ArrayProxy.createWithMixins Em.SortableMixin,
      contentBinding: Em.Binding.oneWay 'App.plans'
      sortProperties: ['totalPrice']

    @setProperties
      name: null
      sortedPlans: sortedPlans
      selectedRegion: App.get('regions.firstObject')
      selectedOrgSlug: null
      error: null
      invalidField: null
      host: null
      saving: false
      templateName: null
      nameFieldEdited: false
      countryDict: null
      freePlanNotAllowed: false
      orgPlans: null
      sortedOrgPlans: null

    if @get('hasPendingContainer')
      @set('name', App.get('user.username'))

    plans?.reload().then =>
      # if user belongs to an org, choose org owner by default
      if @get('orgs.length') > 0
        @set 'selectedOrgSlug', @get('orgs.firstObject.slug')
      else
        @set 'selectedPlan', @get('paidPlans.firstObject')

  hasPendingContainer: (->
    @get('pendingContainer') && App.get('pendingContainerData')
  ).property 'pendingContainer', 'App.pendingContainerData'

  plansDidChange: (->
    unless @get('selectedOrgSlug')
      # update selected plan only if user's plans is updated
      @set('selectedPlan', @get('plans')?.findBy('isPaidPlan'))
  ).observes 'plans.@each'

  regionsDidChange: (->
    unless @get('selectedRegion')
      @set('selectedRegion', App.get('regions.firstObject'))
  ).observes 'App.regions.@each'

  cardRequired: (->
    return false if @get('selectedOrg')
    if App.get('billingProfile.exempt')
      return false

    # otherwise a paid plan must have a card
    if @get('selectedPlan.isPaidPlan')
      return !App.get('billingProfile.hasCard')

    false
  ).property 'App.billingProfile.hasCard', 'selectedPlan.isPaidPlan', 'selectedOrg'

  hasNextStep: (->
    @get('cardRequired')
  ).property 'cardRequired'

  actions:
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
          orgId: @get('selectedOrg.id')
          orgSlug: @get('selectedOrg.slug')
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

        # force a reload of subscriptions to pick up new subscription resulting from a host create
        App.get('subscriptions').reload()

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
