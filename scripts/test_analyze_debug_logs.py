import base64
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from analyze_debug_logs import extract_base64_images  # noqa: E402


PNG_DATA = base64.b64encode(b"png-bytes").decode("ascii")
JPEG_DATA = base64.b64encode(b"jpeg-bytes").decode("ascii")


class ExtractBase64ImagesTest(unittest.TestCase):
    def test_extracts_openai_anthropic_and_gemini_inline_images(self):
        payload = {
            "messages": [{
                "content": [
                    {"type": "image_url", "image_url": {"url": f"data:image/png;base64,{PNG_DATA}"}},
                    {"type": "image", "source": {"type": "base64", "media_type": "image/jpeg", "data": JPEG_DATA}},
                    {"inlineData": {"mimeType": "image/png", "data": PNG_DATA}},
                ],
            }],
        }

        images = extract_base64_images(payload, "input")

        self.assertEqual(len(images), 2, "duplicate image payloads should be rendered once")
        self.assertEqual([image["mime_type"] for image in images], ["image/png", "image/jpeg"])
        self.assertEqual([image["source"] for image in images], ["input", "input"])
        self.assertEqual(images[0]["data"], PNG_DATA)
        self.assertEqual(images[1]["data"], JPEG_DATA)

    def test_rejects_unsupported_or_invalid_image_data(self):
        payload = {
            "svg": "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=",
            "invalid": "data:image/png;base64,not-valid!",
            "wrong_mime": {"media_type": "text/plain", "data": PNG_DATA},
        }

        self.assertEqual(extract_base64_images(payload, "output"), [])

    def test_extracts_openai_image_generation_output(self):
        images = extract_base64_images(
            {"type": "image_generation_call", "result": PNG_DATA},
            "output",
        )

        self.assertEqual(len(images), 1)
        self.assertEqual(images[0]["mime_type"], "image/png")
        self.assertEqual(images[0]["source"], "output")

    def test_infers_mime_for_openai_b64_json_output(self):
        png = base64.b64encode(b"\x89PNG\r\n\x1a\nimage-data").decode("ascii")

        images = extract_base64_images({"data": [{"b64_json": png}]}, "output")

        self.assertEqual(len(images), 1)
        self.assertEqual(images[0]["mime_type"], "image/png")


if __name__ == "__main__":
    unittest.main()
