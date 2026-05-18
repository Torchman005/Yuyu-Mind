from PIL import Image, ImageDraw

def create_placeholder(filename, text, size=(200, 200), color=(100, 100, 255, 128)):
    img = Image.new('RGBA', size, color)
    draw = ImageDraw.Draw(img)
    draw.text((10, 10), text, fill=(255, 255, 255, 255))
    img.save(f"assets/skins/default/{filename}")

if __name__ == "__main__":
    create_placeholder("idle.png", "IDLE")
    create_placeholder("speaking.gif", "SPEAKING")
    create_placeholder("happy.gif", "HAPPY")
    create_placeholder("sleep.png", "SLEEP")
    print("Placeholder assets created.")
