from appslab.modules.objectdetection import ObjectDetection
from appslab.core.image import draw_bounding_boxes
import logging
import gradio as gr
import pandas as pd
import time

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

### Optional gradio styling and theming #
theme = gr.themes.Ocean(
    primary_hue="green",
    secondary_hue="green",
)

js_func = """
function refresh() {
    const url = new URL(window.location);

    if (url.searchParams.get('__theme') !== 'dark') {
        url.searchParams.set('__theme', 'dark');
        window.location.href = url.href;
    }
}
"""
#########################################

# Global variables and settings
object_detection = ObjectDetection()
overlap = 0.4
confidence = 0.25

def process_image(input_image):
    """
    This function takes an image as input (PIL Image object) and performs some processing.
    Replace this with your actual image processing logic.
    """
    try:
        if input_image is None:
            return None, None, None
        
        global object_detection
        global overlap
        global confidence

        start_time = time.time() * 1000
        logger.info(f"Running detection model... overlap:{overlap} confidence:{confidence}")

        results = object_detection.detect(input_image, confidence=confidence, overlap=overlap)

        diff = time.time() * 1000 - start_time
        logger.info(f"Detection completed in {diff:.2f} ms")

        if results is None:
            logger.error("Error processing image: No results returned.")
            return None, None, None

        objects = []
        cfd = []
        count = []
        for i, box in enumerate(results['detection']):
            objects.append(box['class_name'])
            cfd.append(box['confidence'])
            count.append(1)

        # Generate detection classification dataframe
        detection_dataframe = pd.DataFrame(
            {
                "count": count,
                "object": objects,
            }
        )
        detection_dataframe_table = pd.DataFrame(
            {
                "Detected object": objects,
                "Confidence %": cfd,
            }
        )

        img_with_boxes = None
        try:
            img_with_boxes = draw_bounding_boxes(input_image, results)
        except Exception as e:
            logger.error(f"Error drawing bounding boxes: {e}")

        return img_with_boxes, detection_dataframe, detection_dataframe_table
    except Exception as e:
        print(e)
        logger.error(f"Error processing image: {e}")
        return None, None, None

def set_confidence_threshold(val):
    global confidence
    confidence = val

def set_overlap_threshold(val):
    global overlap
    overlap = val

with gr.Blocks(theme=theme,
               title="Arduino vision object detection demo",
               js=js_func,
               delete_cache=(30, 60),
               css="footer{display:none !important}") as built_ui:
    
    gr.HTML(value="<img src='https://upload.wikimedia.org/wikipedia/commons/8/87/Arduino_Logo.svg' width='120px' style='float:right'>", elem_id="arduino_logo")
    gr.Markdown("# Vision object detection with Yolo")

    with gr.Row():
        with gr.Column():
            with gr.Row():
                image_input = gr.Image(label="Input Image", type="pil", streaming=False)

        with gr.Column():
            with gr.Row():   
                confidence_threshold = gr.Slider(0, 1, step=0.05, label="Confidence", value=confidence)
                confidence_threshold.release(set_confidence_threshold, inputs=[confidence_threshold], outputs=None)
            with gr.Row():
                overlap_threshold = gr.Slider(0, 1, step=0.05, label="Overlap", value=overlap)
                overlap_threshold.release(set_overlap_threshold, inputs=[overlap_threshold], outputs=None)
            with gr.Row():
                btn = gr.Button("Run detection")

    gr.HTML("<hr>")

    with gr.Row():
        with gr.Column():
            image_box = gr.Image(label="Object detection results")

            gr.HTML("<hr>")

            object_detection_report = gr.BarPlot(
                x="count",
                y="object",
                y_aggregate="sum",
            )
            
            gr.HTML("<hr>")

            table_summary = gr.DataFrame()

    btn.click(process_image, inputs=image_input, outputs=[image_box, object_detection_report, table_summary])


if __name__ == "__main__":
    built_ui.queue().launch(debug=False, server_name="0.0.0.0", server_port=7860, share=False, favicon_path="python/favicon.ico")
