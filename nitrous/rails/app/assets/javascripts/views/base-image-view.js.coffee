App.BaseImageView = Em.View.extend
  templateName: 'base-image'
  tagName: 'li'
  classNames: ['ui', 'icon-picker']
  classNameBindings: ['isSelected:selected', 'disabled']

  disabled: false

  baseImage: null
  selectedBaseImage: null

  nameBinding: Em.Binding.oneWay 'baseImage.name'

  isSelected: (->
    @get('selectedBaseImage') == @get('baseImage.repo')
  ).property 'selectedBaseImage', 'baseImage.repo'

  iconCss: (->
    return unless iconUrl = @get 'baseImage.iconUrl'
    makeBackgroundImageCss(iconUrl)
  ).property 'baseImage.iconUrl'

  defaultIconCss: (->
    makeBackgroundImageCss(App.BaseImage.defaultIconUrl)
  ).property 'App.BaseImage.defaultIconUrl'

  click: ->
    return if @get 'disabled'
    @set('selectedBaseImage', @get('baseImage.repo'))

makeBackgroundImageCss = (url) ->
  return unless url
  "background-image: url(#{url})"
