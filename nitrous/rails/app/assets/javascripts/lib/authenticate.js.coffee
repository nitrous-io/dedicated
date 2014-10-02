window.App.reopen
  authenticate: (username, password) ->
    new Em.RSVP.Promise (resolve, reject) ->
      $.ajax(
        type: 'POST'
        url: "/api/v0/oauth/token"
        data:
          'grant_type': 'password'
          username: username
          password: password
      ).done((data) ->
        resolve(data.access_token)
      ).fail (jqxhr) ->
        data = jqxhr.responseJSON
        reject(Error.create(type: data.error, message: data.error_description))
