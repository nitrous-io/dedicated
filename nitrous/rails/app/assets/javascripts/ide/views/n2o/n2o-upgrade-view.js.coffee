App.N2oUpgradeView = Em.View.extend
  templateName: 'templates/n2o/upgrade'
  classNames: ['app_page']
  elementId: 'n2o_upgrade_view'

  didInsertElement: ->
    @_super()
    activateMember = App.get('currentUser.aws_activate_plans_enabled')
    App.util.mixpanel 'Viewed N2O Upgrade Page', if activateMember then { aws_activate: true } else {}

  didClickUpgrade: ->
    @get('controller').upgrade()

  formattedDiscount: (->
    "-$#{@get('controller.discount').toFixed(2)}"
  ).property 'controller.discount'

  formattedTotal: (->
    "$#{@get('controller.total').toFixed(2)}"
  ).property 'controller.total'
