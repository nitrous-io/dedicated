#= require ./base-model

App.CurrentUser = klass = App.BaseModel.extend
  username: Em.attr()
  email: Em.attr()
  full_name: Em.attr()
  authentications: Em.attr()
  created_at: Em.attr()
  admin: Em.attr()
  max_n2o: Em.attr(Number)
  used_n2o: Em.attr(Number)
  intro_seen: Em.attr()
  referral_link: Em.attr()
  confirmed_at: Em.attr()
  rewardable_invites_count: Em.attr()
  gravatar_hash: Em.attr()
  subscription_pending: Em.attr()
  coupon: Em.attr()
  team_plans_enabled: Em.attr()
  aws_activate_plans_enabled: Em.attr()
  delinquent_at: Em.attr()
  user_profile: Em.attr()
  user_settings: Em.attr()
  box_snapshots_allowed: Em.attr()
  dedicated_allowed: Em.attr()
  visible_box_count: Em.attr(Number)
  snapshot_count: Em.attr(Number)

  resettingPassword: null
  hasCompany: false

  init: ->
    @set 'admin', @get('max_n2o') ? false
    @set 'max_n2o', @get('max_n2o') ? 0
    @set 'used_n2o', @get('used_n2o') ? 0
    @_super.apply this, arguments

  referralLinkFullURL: (->
    if path = @get 'referral_link'
      "#{location.protocol}//#{location.host}#{path}"
  ).property 'referral_link'

  referralLinkFullShareableURL: (->
    "#{@get 'referralLinkFullURL'}?utm_source=nitrous.io&utm_medium=copypaste&utm_campaign=referral"
  ).property 'referralLinkFullURL'

  referralLinkFullFacebookURL: (->
    "#{@get 'referralLinkFullURL'}?utm_source=nitrous.io&utm_medium=facebook&utm_campaign=referral"
  ).property 'referralLinkFullURL'

  referralLinkFullTwitterURL: (->
    "#{@get 'referralLinkFullURL'}?utm_source=nitrous.io&utm_medium=twitter&utm_campaign=referral"
  ).property 'referralLinkFullURL'

  referralLinkFullGooglePlusURL: (->
    "#{@get 'referralLinkFullURL'}?utm_source=nitrous.io&utm_medium=googleplus&utm_campaign=referral"
  ).property 'referralLinkFullURL'

  gravatarUrl: (->
    if gravatarHash = @get 'gravatar_hash'
      "https://secure.gravatar.com/avatar/#{gravatarHash}.png?s=45"
  ).property 'gravatar_hash'

  isDelinquent: (->
    !!@get('delinquent_at')
  ).property 'delinquent_at'

  hasMoreThanOneBox: (->
    @get('visible_box_count') > 1
  ).property 'visible_box_count'

  update: (properties) ->
    @set 'saving', true
    data =
      user: properties

    $.ajax(
      type: 'PUT'
      url: '/users'
      dataType: 'json'
      data: data
    ).done((json) =>
      if json && json.user && json.user.id
        @load(json.user.id, json.user)
    ).fail((e) =>
      if e.status == 422
        @set 'errors', $.parseJSON(e.responseText).errors
    ).always =>
      @set 'saving', false

  updateInfo: ->
    @update(
      email: @get 'email'
      full_name: @get 'full_name'
      user_profile_attributes: @get 'user_profile'
    )

  updateSettings: ->
    @update(user_settings_attributes: @get('user_settings'))

  resetPassword: ->
    @set 'resettingPassword', true
    $.ajax(
      type: 'POST'
      url: '/users/password'
      dataType: 'json'
      data: { user: { email: @get 'email' } }
    ).fail((e) =>
      if e.status == 422
        @set 'errors', $.parseJSON(e.responseText).errors
    ).always =>
      @set 'resettingPassword', false

  free_n2o: (->
    (@get('max_n2o') ? 0) - (@get('used_n2o') ? 0)
  ).property 'used_n2o', 'max_n2o'
  freeN2O: Em.computed.alias 'free_n2o'

  hasUsedN2O: (->
    @get('used_n2o') > 0
  ).property 'used_n2o'

  hasN2OForNewBox: (->
    @get('free_n2o') >= App.Box.minBoxN2O()
  ).property 'free_n2o'

  isGithubAuthenticated: (->
    _.any @get("authentications"), (authentication) ->
      authentication.provider == "github"
  ).property "authentications"

  isGoogleAuthenticated: (->
    _.any @get("authentications"), (authentication) ->
      authentication.provider == "google"
  ).property "authentications"

  isLinkedinAuthenticated: (->
    _.any @get("authentications"), (authentication) ->
      authentication.provider == "linkedin"
  ).property "authentications"

  isConfirmed: (->
    !!@get("confirmed_at")
  ).property "confirmed_at"

  snapshots: (->
    App.Snapshot.Collection.create
      content: App.get('snapshots').filter (snapshot) =>
        @get('id') == snapshot.get('user_id')
  ).property 'App.snapshots.@each'

  completedSnapshots: (->
    App.Snapshot.Collection.create
      content: App.get('snapshots').filter (snapshot) =>
        @get('id') == snapshot.get('user_id') && snapshot.get('completed')
  ).property 'App.snapshots.@each'

  fetchN2O: ->
    $.ajax(
      type: 'GET'
      dataType: 'json'
      url: '/n2o'
    ).done (json) =>
      if maxN2O = json.n2o.max
        @set 'max_n2o', maxN2O
      if usedN2O = json.n2o.used
        @set 'used_n2o', usedN2O

  fetchCompany: ->
    $.ajax(type: 'GET', dataType: 'json', url: '/user/company')

klass.reopenClass
  adapter: null # do REST manually for this class
  rootKey: 'user'
