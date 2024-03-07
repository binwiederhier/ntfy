import os
import shutil


def on_post_build(config, **kwargs):
    site_dir = config["site_dir"]
    shutil.copytree("docs/static/fonts", os.path.join(site_dir, "get"))
