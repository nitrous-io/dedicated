class TemplatesController < ApplicationController
  def oauth_authorize
    render layout: 'bare'
  end
end
