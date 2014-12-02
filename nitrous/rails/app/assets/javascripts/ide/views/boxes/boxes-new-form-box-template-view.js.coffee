App.BoxesNewFormBoxTemplateView = Em.View.extend
  templateName: 'templates/boxes/new-form-box-template'
  tagName: 'li'
  classNameBindings: ['selected', 'boxTemplate.enabled::coming_soon']

  boxTemplate: null
  box: null

  selected: (->
    @get('boxTemplate.id') == @get('box.box_template_id')
  ).property 'box.box_template_id'

  click: (event) ->
    event.preventDefault() if $(event.target).hasClass('stack-label')
    boxTemplate = @get 'boxTemplate'
    if boxTemplate.get 'enabled'
      @set 'box.box_template_id', boxTemplate.get('id')

