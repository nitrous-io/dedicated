App.AddPhoneRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments

    # Check if user already has an authenticated phone.
    if App.get('user.phoneAuthed')
      @transitionTo 'hosts.new'

  activate: ->
    App.set 'title', 'Add a Phone Number'

  renderTemplate: ->
    @render 'add-phone', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()

    # TODO This should probably come from the backend. Duplicate of map in
    # controllers/users/users.go.
    whitelist = [
      { name: "United States (+1)",   code: "1" }
      { name: "Canada (+1)",          code: "1" }
      { name: "United Kingdom (+44)", code: "44" }
      { name: "Australia (+61)",      code: "61" }
      { name: "New Zealand (+64)",    code: "64" }
      { name: "Germany (+49)",        code: "49" }
      { name: "France (+33)",         code: "33" }
      { name: "Japan (+81)",          code: "81" }
      { name: "Norway (+47)",         code: "47" }
      { name: "Switzerland (+41)",    code: "41" }
      { name: "Netherlands (+31)",    code: "31" }
      { name: "Ireland (+353)",       code: "353" }
      { name: "Austria (+43)",        code: "43" }
      { name: "Sweden (+46)",         code: "46" }
      { name: "Denmark (+45)",        code: "45" }
      { name: "Iceland (+354)",       code: "354" }
      { name: "Belgium (+32)",        code: "32" }
      { name: "Finland (+358)",       code: "358" }
      { name: "Italy (+39)",          code: "39" }
      { name: "Singapore (+65)",      code: "65" }
    ].sort((a, b) ->
      if a.name < b.name
        return -1
      if a.name > b.name
        return 1
      0
    )
    controller.set('whitelist', whitelist)
