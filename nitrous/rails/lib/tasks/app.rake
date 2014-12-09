namespace :build do
  task generate_views: :environment do
    # map of desired html output path to render options
    locations = {
      '/public/app/index.html' => {template: 'web_app/index'},
      '/public/ide/index.html' => {template: 'web_app/ide'},
      '/public/index.html' => {layout: 'layouts/home', template: 'pages/home'},
    }

    build_dir = ENV['BUILD_DIR']
    raise 'Missing BUILD_DIR' if !build_dir

    avb = ActionView::Base.new(Rails::Application::Configuration.new(Rails.root).paths['app/views'])

    locations.each do |path, opts|
      full_path = File.join(build_dir, path)
      FileUtils.mkdir_p(File.dirname(full_path))
      puts "Writing to #{full_path} the results of render with #{opts}"
      File.write(full_path, avb.render(opts))
    end
  end
end
