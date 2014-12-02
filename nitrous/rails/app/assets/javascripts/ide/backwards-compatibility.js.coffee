# Do NOT add any more entries to this file!!!
# Do NOT use this in new code!
App.BackwardsCompatibleRouter = Em.Object.extend
  transitionTo: ->
    router = App.Router.router
    router.transitionTo.apply(router, arguments)

  n2oBonusController: (->
    @getController 'n2oBonus'
  ).property()

  accountController: (->
    @getController 'account'
  ).property()

  boxIdeController: (->
    @getController 'boxIde'
  ).property()

  ideLayoutController: (->
    @getController 'ideLayout'
  ).property()

  ideMainMenuController: (->
    @getController 'ideMainMenu'
  ).property()

  ideSidebarController: (->
    @getController 'ideSidebar'
  ).property()

  ideEditorController: (->
    @getController 'ideEditor'
  ).property()

  ideConsoleController: (->
    @getController 'ideConsole'
  ).property()

  ideCollaborationPaneController: (->
    @getController 'ideCollaborationPane'
  ).property()

  ideAutopartsController: (->
    @getController 'ideAutoparts'
  ).property()

  ideBasicAuthController: (->
    @getController 'ideBasicAuth'
  ).property()

  getController: (name) ->
    App.__container__.lookup("controller:#{name}")

App.set 'router', App.BackwardsCompatibleRouter.create()
