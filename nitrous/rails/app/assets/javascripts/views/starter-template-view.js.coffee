App.StarterTemplateView = Em.View.extend
  templateName: 'base-image'
  tagName: 'li'
  classNames: ['starter-template-view', 'base-image-view']
  classNameBindings: ['isSelected:selected', 'disabled']

  disabled: false

  content: null
  selected: null
  iconUrl: null
  name: null

  isSelected: (->
    @get('selected') == @get('content')
  ).property 'selected', 'content'

  iconCss: (->
    return unless iconUrl = @get 'iconUrl'
    makeBackgroundImageCss(iconUrl)
  ).property 'iconUrl'

  defaultIconCss: (->
    makeBackgroundImageCss(App.BaseImage.defaultIconUrl)
  ).property 'App.BaseImage.defaultIconUrl'

  click: ->
    return if @get 'disabled'
    @set('selected', @get('content'))

makeBackgroundImageCss = (url) ->
  return unless url
  "background-image: url(#{url})"
