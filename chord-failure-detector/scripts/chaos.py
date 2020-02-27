import os
import argparse
import time
import subprocess
import random


def chaos(frequency, n, rounds=100000, restart=False, exclude=[]):
    stopped = []
    to_revive = []

    while rounds:
        containers = (
            subprocess.check_output(["docker", "ps", "-q"]).decode("utf-8").split("\n")
        )
        containers.remove("")
        for e in exclude:
            containers.remove(e)

        random.shuffle(containers)
        victims = containers[0:n]

        # Revive (n // 2) containers every other round
        if restart and rounds % 2 == 0:
            random.shuffle(stopped)
            to_revive = containers[0 : n // 2]

        if not victims:
            print("No valid nodes left to kill!")
        else:
            print(f"Killing: {victims}")
            kill_command = f"docker kill {' '.join(victims)}"
            revive_command = f"docker start {' '.join(to_revive)}"
            os.system(kill_command)

            if restart:
                print(f"Reviving: {to_revive}")
                os.system(revive_command)

            stopped += victims

        rounds -= 1
        if rounds:
            sleep_time = random.randint(1, frequency)
            print(f"Next round in {sleep_time} seconds. {rounds} round(s) remaining")
            time.sleep(sleep_time)
        else:
            print(f"Done!")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-f", "--frequency", type=int, help="frequency", required=True)
    parser.add_argument("-n", "--number", type=int, help="number of nodes", required=True)
    parser.add_argument(
        "-r",
        "--restart",
        action="store_true",
        help="whether to randomly restart nodes as well",
    )
    parser.add_argument(
        "-e", "--exclude", nargs="+", help="container IDs to leave alone", default=""
    )
    parser.add_argument("--rounds", type=int, help="number of rounds")

    args = parser.parse_args()
    chaos(args.frequency, args.number, args.rounds, args.restart, args.exclude)
