#= require ./model
attr = SM.attr

App.UserProfile = klass = App.Model.extend
  id: attr('number', serialize: false)
  advanced: attr('boolean', default: false)
  maxHosts: attr('number', as: 'max_hosts', serialize: false)
  authorizedGitHubUserName: attr(as: 'authorized_github_user_name', serialize: false)

  githubAuthorize: ->
    Em.$.ajax(
      type: 'POST'
      url: '/api/v0/github/authorize'
      cache: false
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    ).done((json) ->
      window.location.href = json.redirect_url
    ).fail (jqxhr) ->
      App.postError(jqxhr.responseJSON, 'Failed to authorize!')

klass.reopenClass
  url: '/user/profile'
  primaryKey: 'id'
  singular: true
  rootKey: 'user_profile'
