import json
from dataclasses import dataclass
from time import sleep

import requests


@dataclass
class ISSLocation:
    latitude: float
    longitude: float


def get_iss_location():
    """Fetches and prints the current location of the ISS."""
    try:
        response = requests.get("http://api.open-notify.org/iss-now.json")
        response.raise_for_status()  # Raise HTTPError for bad responses (4xx or 5xx)

        data = response.json()
        iss_position = data["iss_position"]
        latitude = float(iss_position["latitude"])
        longitude = float(iss_position["longitude"])

        return ISSLocation(latitude, longitude)

    except requests.exceptions.RequestException as e:
        raise Exception(f"Error fetching ISS location: {e}")
    except json.JSONDecodeError as e:
        raise Exception(f"Error decoding JSON response: {e}")
    except KeyError as e:
        raise Exception(f"Error accessing data in JSON: {e}")
    except Exception as e:
        raise Exception(f"An error occurred: {e}")


def main():
    while True:
        try:
            location = get_iss_location()
            print(
                f"The ISS is currently at {location.latitude}, {location.longitude}",
                flush=True,
            )

            sleep(1)
        except KeyboardInterrupt:
            print("Exiting...")
            break


if __name__ == "__main__":
    main()
