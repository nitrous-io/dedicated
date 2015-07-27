App.AdminTemplatesController = Em.Controller.extend
  actions:
    new: ->
      @transitionToRoute 'admin.templates-new'

    edit: (template) ->
      @transitionToRoute 'admin.template-edit', template.get('slug')
