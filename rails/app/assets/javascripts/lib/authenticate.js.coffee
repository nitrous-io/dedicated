window.App.reopen
  authenticate: (username, password, cb) -> # cb(err, token)
    $.ajax(
      type: 'POST'
      url: "/api/v0/oauth/token"
      data:
        'grant_type': 'password'
        username: username
        password: password
    ).done((data) ->
      cb null, data.access_token
    ).fail (jqxhr) ->
      data = jqxhr.responseJSON
      cb Error.create(type: data.error, message: data.error_description)
